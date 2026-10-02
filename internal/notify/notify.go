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

// ErrNotificationPoison means a malformed notification is waiting for a
// bounded redelivery attempt. Once the durable retry limit is reached, the
// row is skipped and audited.
var ErrNotificationPoison = errors.New("notification poison pending retry")

// ErrEventExpired means an event older than the notification deduplication
// horizon cannot be reintroduced after retention pruning.
var ErrEventExpired = errors.New("notification event expired")

const notificationRetentionFloor = 7 * 24 * time.Hour

const maxNotificationPoisonAttempts = 3

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
	traceparent := nullableString(event.Traceparent)
	tracestate := nullableString(event.Tracestate)
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
		INSERT INTO notifications (tenant_id, cursor, event_id, event_type, event_json, event_sha256, traceparent, tracestate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(tenant_id, event_id) DO NOTHING`,
		event.TenantID, cursor, event.ID, event.Type, eventJSON, eventSHA[:], traceparent, tracestate, now.UTC().Format(time.RFC3339Nano))
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

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// ReadPage returns up to limit events strictly after cursor. It refuses a
// resume cursor that predates retained data, and when maxLag is positive it
// disconnects a subscriber lagging further behind; both refusals are audited.
// A record that fails its digest, decoding, or validation is a poison record:
// it is skipped once its retry budget is spent and otherwise fails the page.
func ReadPage(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int, maxLag int64, now time.Time) (Page, error) {
	if limit <= 0 || limit > 1000 {
		return Page{}, fmt.Errorf("notification limit must be between 1 and 1000")
	}
	if err := checkResumeCursor(ctx, db, tenantID, cursor, maxLag, now); err != nil {
		return Page{}, err
	}
	rawRecords, err := readRawNotifications(ctx, db, tenantID, cursor, limit)
	if err != nil {
		return Page{}, err
	}
	result := Page{Records: make([]Record, 0, limit), NextCursor: cursor}
	for _, raw := range rawRecords {
		result.NextCursor = raw.cursor
		record, valid := raw.decode(tenantID)
		if !valid {
			skip, poisonErr := recordPoisonAttempt(ctx, db, tenantID, raw.cursor, now)
			if poisonErr != nil {
				return Page{}, poisonErr
			}
			if !skip {
				return Page{}, ErrNotificationPoison
			}
			result.Skipped++
			continue
		}
		if err := clearPoisonAttempt(ctx, db, tenantID, raw.cursor); err != nil {
			return Page{}, err
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

// checkResumeCursor refuses an expired cursor or, when maxLag is positive, a
// subscriber that lags too far behind, auditing each refusal.
func checkResumeCursor(ctx context.Context, db *storage.DB, tenantID string, cursor, maxLag int64, now time.Time) error {
	var oldest sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT MIN(cursor) FROM notifications WHERE tenant_id = ?", tenantID).Scan(&oldest); err != nil {
		return fmt.Errorf("find oldest notification: %w", err)
	}
	var nextCursor sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT next_cursor FROM notification_cursors WHERE tenant_id = ?", tenantID).Scan(&nextCursor); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read notification highwater: %w", err)
	}
	if (oldest.Valid && cursor < oldest.Int64-1) || (!oldest.Valid && nextCursor.Valid && cursor < nextCursor.Int64-1) {
		if err := audit(ctx, db, tenantID, "cursor_expired", cursor, oldest.Int64, now); err != nil {
			return fmt.Errorf("audit expired cursor: %w", err)
		}
		return ErrCursorExpired
	}
	if maxLag > 0 && nextCursor.Valid && nextCursor.Int64-1-cursor > maxLag {
		if err := audit(ctx, db, tenantID, "subscriber_too_slow", cursor, oldest.Int64, now); err != nil {
			return fmt.Errorf("audit slow subscriber: %w", err)
		}
		return ErrSubscriberTooSlow
	}
	return nil
}

type rawNotification struct {
	cursor              int64
	eventJSON, eventSHA []byte
}

// readRawNotifications reads the page rows and closes the result set before
// poison accounting writes to the database.
func readRawNotifications(ctx context.Context, db *storage.DB, tenantID string, cursor int64, limit int) ([]rawNotification, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT cursor, event_json, event_sha256 FROM notifications
		WHERE tenant_id = ? AND cursor > ? ORDER BY cursor LIMIT ?`, tenantID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	rawRecords := make([]rawNotification, 0, limit)
	for rows.Next() {
		var raw rawNotification
		if err := rows.Scan(&raw.cursor, &raw.eventJSON, &raw.eventSHA); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		rawRecords = append(rawRecords, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notifications: %w", err)
	}
	return rawRecords, nil
}

// decode verifies the stored digest, then decodes and validates the event.
func (raw rawNotification) decode(tenantID string) (Record, bool) {
	record := Record{TenantID: tenantID, Cursor: raw.cursor}
	computed := sha256.Sum256(raw.eventJSON)
	if !bytes.Equal(computed[:], raw.eventSHA) {
		return Record{}, false
	}
	if err := json.Unmarshal(raw.eventJSON, &record.Event); err != nil {
		return Record{}, false
	}
	if err := record.Event.Validate(); err != nil {
		return Record{}, false
	}
	return record, true
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
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return auditTx(ctx, tx, tenantID, action, requested, oldest, now) }); err != nil {
		return fmt.Errorf("write notification audit: %w", err)
	}
	return nil
}

func auditTx(ctx context.Context, tx *sql.Tx, tenantID, action string, requested, oldest int64, now time.Time) error {
	details, _ := canonicaljson.Marshal(map[string]any{"requested_cursor": requested, "oldest_cursor": oldest})
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_audits (audit_id, tenant_id, action, requested_cursor, oldest_cursor, details_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, ids.Random().New(ids.PrefixPolicy), tenantID, action, requested, oldest, details, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("write notification audit: %w", err)
	}
	return nil
}

func recordPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64, now time.Time) (bool, error) {
	var skip bool
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO notification_poison_attempts (tenant_id, cursor, attempts, last_attempt_at) VALUES (?, ?, 1, ?) ON CONFLICT(tenant_id, cursor) DO UPDATE SET attempts = attempts + 1, last_attempt_at = excluded.last_attempt_at`, tenantID, cursor, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("record notification poison attempt: %w", err)
		}
		var attempts int
		if err := tx.QueryRowContext(ctx, "SELECT attempts FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor).Scan(&attempts); err != nil {
			return fmt.Errorf("read notification poison attempts: %w", err)
		}
		if attempts < maxNotificationPoisonAttempts {
			return nil
		}
		if err := auditTx(ctx, tx, tenantID, "subscriber_skipped", cursor, cursor, now); err != nil {
			return fmt.Errorf("audit skipped poison notification: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
			return fmt.Errorf("clear notification poison attempts: %w", err)
		}
		skip = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("persist notification poison attempt: %w", err)
	}
	return skip, nil
}

func clearPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
		return fmt.Errorf("clear notification poison attempt: %w", err)
	}
	return nil
}
