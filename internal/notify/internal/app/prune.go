package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
)

// Prune deletes only notifications older than the requested retention period;
// the minimum is seven days and notification_cursors preserves monotonicity.
// Retired identities stay as tombstones for the deduplication horizon.
func (s *Service) Prune(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	if err := domain.CheckRetention(retention); err != nil {
		return 0, err
	}
	cutoff := domain.RetentionCutoff(now, retention)
	tx := s.store.Autocommit()
	if err := tx.RetireNotifications(ctx, now, cutoff); err != nil {
		return 0, err
	}
	deleted, err := tx.DeleteRetiredNotifications(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	return deleted, tx.DeleteExpiredTombstones(ctx, cutoff)
}
