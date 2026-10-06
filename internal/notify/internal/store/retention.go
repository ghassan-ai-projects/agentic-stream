package store

import (
	"context"
	"fmt"
	"time"
)

// RetireNotifications records a tombstone for every notification created
// before cutoff.
func (tx *Tx) RetireNotifications(ctx context.Context, now, cutoff time.Time) error {
	if _, err := tx.q.ExecContext(ctx, `INSERT INTO notification_event_tombstones (tenant_id, event_id, event_sha256, retired_at) SELECT tenant_id, event_id, event_sha256, ? FROM notifications WHERE created_at < ? ON CONFLICT(tenant_id, event_id) DO NOTHING`, formatTime(now), formatTime(cutoff)); err != nil {
		return fmt.Errorf("tombstone notifications: %w", err)
	}
	return nil
}

// DeleteRetiredNotifications removes notifications created before cutoff and
// returns how many it removed.
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

// DeleteExpiredTombstones removes tombstones retired before cutoff.
func (tx *Tx) DeleteExpiredTombstones(ctx context.Context, cutoff time.Time) error {
	if _, err := tx.q.ExecContext(ctx, "DELETE FROM notification_event_tombstones WHERE retired_at < ?", formatTime(cutoff)); err != nil {
		return fmt.Errorf("prune notification tombstones: %w", err)
	}
	return nil
}
