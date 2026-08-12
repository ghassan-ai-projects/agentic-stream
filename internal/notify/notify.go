// Package notify provides durable, cursor-resumable notification delivery.
package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ErrCursorExpired means the requested cursor is older than retained data;
// the caller must perform an audited resnapshot before resuming.
var ErrCursorExpired = errors.New("notification cursor expired")

// ErrSubscriberTooSlow means the subscriber exceeded its bounded backlog.
var ErrSubscriberTooSlow = errors.New("subscriber too slow")

// ErrEventExpired means an event older than the notification deduplication
// horizon cannot be reintroduced after retention pruning.
var ErrEventExpired = errors.New("notification event expired")

const notificationRetentionFloor = 7 * 24 * time.Hour

// Record is one durable notification and its tenant-local cursor.
type Record struct {
	TenantID string
	Cursor   int64
	Event    contractsv1.CloudEvent
}

// Page is a bounded delivery result. NextCursor advances past skipped poison
// rows so a consumer cannot stall forever on one corrupt record.
type Page struct {
	Records    []Record
	NextCursor int64
	Skipped    int
}

// Append validates and appends a CloudEvent in the caller's transaction. Use
// this transaction together with the state mutation that caused the event.
func Append(ctx context.Context, tx *sql.Tx, event contractsv1.CloudEvent, now time.Time) (int64, error) {
	if err := event.Validate(); err != nil {
		return 0, fmt.Errorf("validate notification: %w", err)
	}
	if event.Time.Before(now.Add(-notificationRetentionFloor)) {
		return 0, ErrEventExpired
	}
	eventJSON, err := canonicaljson.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("canonicalize notification: %w", err)
	}
	eventSHA := sha256.Sum256(eventJSON)
	var existingCursor int64
	var existingSHA []byte
	if err := tx.QueryRowContext(ctx, "SELECT cursor, event_sha256 FROM notifications WHERE tenant_id = ? AND event_id = ?", event.TenantID, event.ID).Scan(&existingCursor, &existingSHA); err == nil {
		if !bytes.Equal(existingSHA, eventSHA[:]) {
			return 0, fmt.Errorf("notification event id %q has conflicting payload", event.ID)
		}
		return existingCursor, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check duplicate notification: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT event_sha256 FROM notification_event_tombstones WHERE tenant_id = ? AND event_id = ?", event.TenantID, event.ID).Scan(&existingSHA); err == nil {
		if !bytes.Equal(existingSHA, eventSHA[:]) {
			return 0, fmt.Errorf("notification event id %q conflicts with tombstone", event.ID)
		}
		return 0, fmt.Errorf("notification event %q was already retired", event.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check notification tombstone: %w", err)
	}
	var cursor int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO notification_cursors (tenant_id, next_cursor) VALUES (?, 2)
		ON CONFLICT(tenant_id) DO UPDATE SET next_cursor = next_cursor + 1
		RETURNING next_cursor - 1`, event.TenantID).Scan(&cursor); err != nil {
		return 0, fmt.Errorf("allocate notification cursor: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO notifications (tenant_id, cursor, event_id, event_type, event_json, event_sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(tenant_id, event_id) DO NOTHING`,
		event.TenantID, cursor, event.ID, event.Type, eventJSON, eventSHA[:], now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("append notification: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var storedSHA []byte
		if err := tx.QueryRowContext(ctx, "SELECT event_sha256 FROM notifications WHERE tenant_id = ? AND event_id = ?", event.TenantID, event.ID).Scan(&storedSHA); err != nil {
			return 0, fmt.Errorf("read duplicate notification: %w", err)
		}
		if !bytes.Equal(storedSHA, eventSHA[:]) {
			return 0, fmt.Errorf("notification event id %q has conflicting payload", event.ID)
		}
		rollback, rollbackErr := tx.ExecContext(ctx, "UPDATE notification_cursors SET next_cursor = next_cursor - 1 WHERE tenant_id = ? AND next_cursor = ?", event.TenantID, cursor+1)
		if rollbackErr != nil {
			return 0, fmt.Errorf("rollback duplicate cursor: %w", rollbackErr)
		}
		if affected, _ := rollback.RowsAffected(); affected != 1 {
			return 0, fmt.Errorf("rollback duplicate cursor lost race")
		}
	}
	var actual int64
	if err := tx.QueryRowContext(ctx, "SELECT cursor FROM notifications WHERE tenant_id = ? AND event_id = ?", event.TenantID, event.ID).Scan(&actual); err != nil {
		return 0, fmt.Errorf("read notification cursor: %w", err)
	}
	return actual, nil
}

// ReadAfter returns up to limit events strictly after cursor. It refuses a
// resume cursor that predates retained data and records an audit for that
// refusal.
// ReadPage reads a bounded page. maxLag of zero disables the slow-subscriber
// check; otherwise a lagging subscriber is audited and disconnected.
func ReadPage(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int, maxLag int64, now time.Time) (Page, error) {
	if limit <= 0 || limit > 1000 {
		return Page{}, fmt.Errorf("notification limit must be between 1 and 1000")
	}
	var oldest sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT MIN(cursor) FROM notifications WHERE tenant_id = ?", tenantID).Scan(&oldest); err != nil {
		return Page{}, fmt.Errorf("find oldest notification: %w", err)
	}
	var nextCursor sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT next_cursor FROM notification_cursors WHERE tenant_id = ?", tenantID).Scan(&nextCursor); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Page{}, fmt.Errorf("read notification highwater: %w", err)
	}
	if (oldest.Valid && cursor < oldest.Int64-1) || (!oldest.Valid && nextCursor.Valid && cursor < nextCursor.Int64-1) {
		oldestCursor := int64(0)
		if oldest.Valid {
			oldestCursor = oldest.Int64
		}
		if err := audit(ctx, db, tenantID, "cursor_expired", cursor, oldestCursor, now); err != nil {
			return Page{}, fmt.Errorf("audit expired cursor: %w", err)
		}
		return Page{}, ErrCursorExpired
	}
	if maxLag > 0 && nextCursor.Valid && nextCursor.Int64-1-cursor > maxLag {
		if err := audit(ctx, db, tenantID, "subscriber_too_slow", cursor, oldest.Int64, now); err != nil {
			return Page{}, fmt.Errorf("audit slow subscriber: %w", err)
		}
		return Page{}, ErrSubscriberTooSlow
	}
	rows, err := db.QueryContext(ctx, `
		SELECT cursor, event_json, event_sha256 FROM notifications
		WHERE tenant_id = ? AND cursor > ? ORDER BY cursor LIMIT ?`, tenantID, cursor, limit)
	if err != nil {
		return Page{}, fmt.Errorf("read notifications: %w", err)
	}
	result := Page{Records: make([]Record, 0, limit), NextCursor: cursor}
	type rawRecord struct {
		cursor              int64
		eventJSON, eventSHA []byte
	}
	rawRecords := make([]rawRecord, 0, limit)
	for rows.Next() {
		var record Record
		var eventJSON, eventSHA []byte
		record.TenantID = tenantID
		if err := rows.Scan(&record.Cursor, &eventJSON, &eventSHA); err != nil {
			_ = rows.Close()
			return Page{}, fmt.Errorf("scan notification: %w", err)
		}
		rawRecords = append(rawRecords, rawRecord{record.Cursor, eventJSON, eventSHA})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Page{}, fmt.Errorf("iterate notifications: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Page{}, fmt.Errorf("close notifications: %w", err)
	}
	for _, raw := range rawRecords {
		record := Record{TenantID: tenantID, Cursor: raw.cursor}
		eventJSON, eventSHA := raw.eventJSON, raw.eventSHA
		result.NextCursor = record.Cursor
		computed := sha256.Sum256(eventJSON)
		if !bytes.Equal(computed[:], eventSHA) {
			if err := audit(ctx, db, tenantID, "subscriber_skipped", record.Cursor, record.Cursor, now); err != nil {
				return Page{}, fmt.Errorf("audit corrupt notification: %w", err)
			}
			result.Skipped++
			continue
		}
		if err := json.Unmarshal(eventJSON, &record.Event); err != nil {
			if err := audit(ctx, db, tenantID, "subscriber_skipped", record.Cursor, record.Cursor, now); err != nil {
				return Page{}, fmt.Errorf("audit poison notification: %w", err)
			}
			result.Skipped++
			continue
		}
		if err := record.Event.Validate(); err != nil {
			if err := audit(ctx, db, tenantID, "subscriber_skipped", record.Cursor, record.Cursor, now); err != nil {
				return Page{}, fmt.Errorf("audit invalid notification: %w", err)
			}
			result.Skipped++
			continue
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

// Prune deletes only notifications older than the requested retention period;
// the minimum is seven days and notification_cursors preserves monotonicity.
func Prune(ctx context.Context, db *storage.DB, now time.Time, retention time.Duration) (int64, error) {
	if retention < notificationRetentionFloor {
		return 0, fmt.Errorf("notification retention cannot be shorter than seven days")
	}
	cutoff := now.Add(-retention).UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_event_tombstones (tenant_id, event_id, event_sha256, retired_at) SELECT tenant_id, event_id, event_sha256, ? FROM notifications WHERE created_at < ? ON CONFLICT(tenant_id, event_id) DO NOTHING`, now.UTC().Format(time.RFC3339Nano), cutoff); err != nil {
		return 0, fmt.Errorf("tombstone notifications: %w", err)
	}
	result, err := db.ExecContext(ctx, "DELETE FROM notifications WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune notifications: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned notifications: %w", err)
	}
	// Tombstones share the bounded deduplication horizon. Replay beyond the
	// retention contract is already refused by cursor expiry.
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_event_tombstones WHERE retired_at < ?", cutoff); err != nil {
		return 0, fmt.Errorf("prune notification tombstones: %w", err)
	}
	return deleted, nil
}

func audit(ctx context.Context, db *storage.DB, tenantID, action string, requested, oldest int64, now time.Time) error {
	details, _ := canonicaljson.Marshal(map[string]any{"requested_cursor": requested, "oldest_cursor": oldest})
	_, err := db.ExecContext(ctx, `INSERT INTO notification_audits (audit_id, tenant_id, action, requested_cursor, oldest_cursor, details_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ids.Random().New(ids.PrefixPolicy), tenantID, action, requested, oldest, details, now.UTC().Format(time.RFC3339Nano))
	return err
}
