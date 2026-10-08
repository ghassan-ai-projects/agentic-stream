package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Stored is an identity already present in the outbox or its tombstones.
type Stored struct {
	Cursor int64
	SHA    []byte
}

// Notification is one row to insert.
type Notification struct {
	TenantID    string
	EventID     string
	EventType   string
	Cursor      int64
	EventJSON   []byte
	EventSHA    []byte
	Traceparent string
	Tracestate  string
	CreatedAt   time.Time
}

// FindNotification returns the stored notification for an event identity.
func (tx *Tx) FindNotification(ctx context.Context, tenantID, eventID string) (Stored, bool, error) {
	var stored Stored
	err := tx.q.QueryRowContext(ctx, "SELECT cursor, event_sha256 FROM notifications WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&stored.Cursor, &stored.SHA)
	if errors.Is(err, sql.ErrNoRows) {
		return Stored{}, false, nil
	}
	if err != nil {
		return Stored{}, false, fmt.Errorf("check duplicate notification: %w", err)
	}
	return stored, true, nil
}

// FindTombstone returns the retired SHA-256 of an event identity.
func (tx *Tx) FindTombstone(ctx context.Context, tenantID, eventID string) ([]byte, bool, error) {
	var retired []byte
	err := tx.q.QueryRowContext(ctx, "SELECT event_sha256 FROM notification_event_tombstones WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&retired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("check notification tombstone: %w", err)
	}
	return retired, true, nil
}

// AllocateCursor takes the tenant's next cursor.
func (tx *Tx) AllocateCursor(ctx context.Context, tenantID string) (int64, error) {
	var cursor int64
	if err := tx.q.QueryRowContext(ctx, `
		INSERT INTO notification_cursors (tenant_id, next_cursor) VALUES (?, 2)
		ON CONFLICT(tenant_id) DO UPDATE SET next_cursor = next_cursor + 1
		RETURNING next_cursor - 1`, tenantID).Scan(&cursor); err != nil {
		return 0, fmt.Errorf("allocate notification cursor: %w", err)
	}
	return cursor, nil
}

// InsertNotification stores the row and reports false when an identical event
// identity was already stored.
func (tx *Tx) InsertNotification(ctx context.Context, n Notification) (bool, error) {
	result, err := tx.q.ExecContext(ctx, `
		INSERT INTO notifications (tenant_id, cursor, event_id, event_type, event_json, event_sha256, traceparent, tracestate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(tenant_id, event_id) DO NOTHING`,
		n.TenantID, n.Cursor, n.EventID, n.EventType, n.EventJSON, n.EventSHA, storage.NullIfEmpty(n.Traceparent), storage.NullIfEmpty(n.Tracestate), sources.FormatTime(n.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("append notification: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected != 0, nil
}

// StoredSHA reads the SHA-256 of the notification that won a concurrent insert.
func (tx *Tx) StoredSHA(ctx context.Context, tenantID, eventID string) ([]byte, error) {
	var stored []byte
	if err := tx.q.QueryRowContext(ctx, "SELECT event_sha256 FROM notifications WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&stored); err != nil {
		return nil, fmt.Errorf("read duplicate notification: %w", err)
	}
	return stored, nil
}

// ReleaseCursor gives back the cursor allocated for an insert that a
// concurrent identical append won, keeping cursors gapless.
func (tx *Tx) ReleaseCursor(ctx context.Context, tenantID string, cursor int64) error {
	rollback, err := tx.q.ExecContext(ctx, "UPDATE notification_cursors SET next_cursor = next_cursor - 1 WHERE tenant_id = ? AND next_cursor = ?", tenantID, cursor+1)
	if err != nil {
		return fmt.Errorf("rollback duplicate cursor: %w", err)
	}
	if affected, _ := rollback.RowsAffected(); affected != 1 {
		return fmt.Errorf("rollback duplicate cursor lost race")
	}
	return nil
}

// NotificationCursor reads the cursor stored for an event identity.
func (tx *Tx) NotificationCursor(ctx context.Context, tenantID, eventID string) (int64, error) {
	var cursor int64
	if err := tx.q.QueryRowContext(ctx, "SELECT cursor FROM notifications WHERE tenant_id = ? AND event_id = ?", tenantID, eventID).Scan(&cursor); err != nil {
		return 0, fmt.Errorf("read notification cursor: %w", err)
	}
	return cursor, nil
}
