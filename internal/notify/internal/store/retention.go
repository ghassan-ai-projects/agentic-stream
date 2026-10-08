package store

import (
	"context"
	"fmt"
	"time"
)

func (tx *Tx) RetireNotifications(ctx context.Context, now, cutoff time.Time) error {
	if _, err := tx.q.ExecContext(ctx, `INSERT INTO notification_event_tombstones (tenant_id, event_id, event_sha256, retired_at) SELECT tenant_id, event_id, event_sha256, ? FROM notifications WHERE created_at < ? ON CONFLICT(tenant_id, event_id) DO NOTHING`, formatTime(now), formatTime(cutoff)); err != nil {
		return fmt.Errorf("tombstone notifications: %w", err)
	}
	return nil
}

func (tx *Tx) DeleteRetiredNotifications(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := tx.q.ExecContext(ctx, "DELETE FROM notifications WHERE created_at < ?", formatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("prune notifications: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned notifications: %w", err)
	}
	return deleted, nil
}

func (tx *Tx) DeleteExpiredTombstones(ctx context.Context, cutoff time.Time) error {
	if _, err := tx.q.ExecContext(ctx, "DELETE FROM notification_event_tombstones WHERE retired_at < ?", formatTime(cutoff)); err != nil {
		return fmt.Errorf("prune notification tombstones: %w", err)
	}
	return nil
}

func (tx *Tx) CountNotificationsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	if err := tx.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE created_at < ?", formatTime(cutoff)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count retirable notifications: %w", err)
	}
	return count, nil
}
