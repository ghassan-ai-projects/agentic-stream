package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Bounds is what the outbox holds for a tenant.
type Bounds struct {
	Oldest        int64
	HasOldest     bool
	NextCursor    int64
	HasNextCursor bool
}

// Row is one stored notification as read, before its digest is verified.
type Row struct {
	Cursor    int64
	EventJSON []byte
	EventSHA  []byte
}

const readPageSQL = `
 SELECT cursor, event_json, event_sha256 FROM notifications
 WHERE tenant_id = ? AND cursor > ? ORDER BY cursor LIMIT ?`

// TenantBounds reads the tenant's oldest retained cursor and its highwater.
func (tx *Tx) TenantBounds(ctx context.Context, tenantID string) (Bounds, error) {
	var oldest, next sql.NullInt64
	if err := tx.q.QueryRowContext(ctx, "SELECT MIN(cursor) FROM notifications WHERE tenant_id = ?", tenantID).Scan(&oldest); err != nil {
		return Bounds{}, fmt.Errorf("find oldest notification: %w", err)
	}
	if err := tx.q.QueryRowContext(ctx, "SELECT next_cursor FROM notification_cursors WHERE tenant_id = ?", tenantID).Scan(&next); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Bounds{}, fmt.Errorf("read notification highwater: %w", err)
	}
	return Bounds{Oldest: oldest.Int64, HasOldest: oldest.Valid, NextCursor: next.Int64, HasNextCursor: next.Valid}, nil
}

// ReadRows reads up to limit rows strictly after cursor and closes the result
// set before the caller writes anything.
func (tx *Tx) ReadRows(ctx context.Context, tenantID string, cursor int64, limit int) ([]Row, error) {
	rows, err := tx.q.QueryContext(ctx, readPageSQL, tenantID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := collectRows(rows, limit)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notifications: %w", err)
	}
	return records, nil
}

func collectRows(rows *sql.Rows, limit int) ([]Row, error) {
	records := make([]Row, 0, limit)
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.Cursor, &row.EventJSON, &row.EventSHA); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return records, nil
}
