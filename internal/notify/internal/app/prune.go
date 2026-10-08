package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

func (s *Service) Prune(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	if err := domain.CheckRetention(retention); err != nil {
		return 0, fmt.Errorf("check retention: %w", err)
	}
	cutoff := domain.RetentionCutoff(now, retention)
	var deleted int64
	err := s.store.WithTx(ctx, func(tx *store.Tx) (err error) {
		deleted, err = retire(ctx, tx, now, cutoff)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("prune notifications before %s: %w", cutoff.Format(time.RFC3339), err)
	}
	return deleted, nil
}

func retire(ctx context.Context, tx *store.Tx, now, cutoff time.Time) (int64, error) {
	if err := tx.RetireNotifications(ctx, now, cutoff); err != nil {
		return 0, fmt.Errorf("retire notifications: %w", err)
	}
	deleted, err := tx.DeleteRetiredNotifications(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("remove retired notifications: %w", err)
	}
	if err := tx.DeleteExpiredTombstones(ctx, cutoff); err != nil {
		return 0, fmt.Errorf("remove expired tombstones: %w", err)
	}
	return deleted, nil
}

func (s *Service) Prunable(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	if err := domain.CheckRetention(retention); err != nil {
		return 0, fmt.Errorf("check retention: %w", err)
	}
	count, err := s.store.Autocommit().CountNotificationsBefore(ctx, domain.RetentionCutoff(now, retention))
	if err != nil {
		return 0, fmt.Errorf("count notifications before the retention cutoff: %w", err)
	}
	return count, nil
}
