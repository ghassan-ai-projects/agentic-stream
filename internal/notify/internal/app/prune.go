package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// Prune deletes only notifications older than the requested retention period;
// the minimum is seven days and notification_cursors preserves monotonicity.
// Retired identities stay as tombstones for the deduplication horizon. The
// three steps commit together: a failure leaves nothing half retired.
func (s *Service) Prune(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	if err := domain.CheckRetention(retention); err != nil {
		return 0, err
	}
	cutoff := domain.RetentionCutoff(now, retention)
	var deleted int64
	err := s.store.WithTx(ctx, func(tx *store.Tx) (err error) {
		deleted, err = retire(ctx, tx, now, cutoff)
		return err
	})
	return deleted, err //nolint:wrapcheck // The store names the failed step.
}

func retire(ctx context.Context, tx *store.Tx, now, cutoff time.Time) (int64, error) {
	if err := tx.RetireNotifications(ctx, now, cutoff); err != nil {
		return 0, err //nolint:wrapcheck // The store names the failed step.
	}
	deleted, err := tx.DeleteRetiredNotifications(ctx, cutoff)
	if err != nil {
		return 0, err //nolint:wrapcheck // The store names the failed step.
	}
	return deleted, tx.DeleteExpiredTombstones(ctx, cutoff) //nolint:wrapcheck // The store names the failed step.
}

// Prunable counts the notifications Prune would retire, changing nothing.
func (s *Service) Prunable(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	if err := domain.CheckRetention(retention); err != nil {
		return 0, err
	}
	return s.store.Autocommit().CountNotificationsBefore(ctx, domain.RetentionCutoff(now, retention)) //nolint:wrapcheck // The store names the failed read.
}
