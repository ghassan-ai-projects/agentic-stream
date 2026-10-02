// Package notify provides durable, cursor-resumable notification delivery.
package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
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
