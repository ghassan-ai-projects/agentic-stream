package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// Expire marks due active watches expired without deleting their audit rows.
func (s *Service) Expire(ctx context.Context) error {
	now := s.clk.Now().UTC()
	return retryWhileContended(ctx, func() error { return s.expireDue(ctx, now) })
}

// expireDue marks every active watch whose expiry has passed as expired.
func (s *Service) expireDue(ctx context.Context, now time.Time) error {
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.AssertOwner(ctx); err != nil {
			return err
		}
		return tx.ExpireDue(ctx, now)
	}); err != nil {
		return fmt.Errorf("expire watch transaction: %w", err)
	}
	return nil
}

// retryWhileContended retries expiry up to domain.ExpireAttempts times, waiting
// domain.ExpireBackoff between attempts, while SQLite reports writer contention.
func retryWhileContended(ctx context.Context, expire func() error) error {
	var err error
	for attempt := 0; attempt < domain.ExpireAttempts; attempt++ {
		if err = expire(); err == nil {
			return nil
		}
		if !store.IsContended(err) || attempt == domain.ExpireAttempts-1 {
			break
		}
		if err := awaitRetry(ctx); err != nil {
			return err
		}
	}
	return fmt.Errorf("expire watch conditions: %w", err)
}

func awaitRetry(ctx context.Context) error {
	timer := time.NewTimer(domain.ExpireBackoff)
	select {
	case <-ctx.Done():
		timer.Stop()
		return fmt.Errorf("expire watch conditions: wait for retry: %w", ctx.Err())
	case <-timer.C:
	}
	return nil
}
