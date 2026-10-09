package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// Expire marks due active watches expired without deleting their audit rows.
func (s *Service) Expire(ctx context.Context) error {
	now := s.clk.Now().UTC()
	if err := s.store.RetryBusy(ctx, func() error { return s.expireDue(ctx, now) }); err != nil {
		return fmt.Errorf("expire watch conditions: %w", err)
	}
	return nil
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
