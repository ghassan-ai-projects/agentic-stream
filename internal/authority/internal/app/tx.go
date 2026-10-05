package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// inAdmittedTx runs work in one unit of work whose first step is ordinary
// admission for epoch, so the decision and its writes share the fence.
func (s *Service) inAdmittedTx(ctx context.Context, epoch string, work func(*store.Tx) error) error {
	return s.store.InTx(ctx, func(tx *store.Tx) error {
		if err := s.admitOrdinary(ctx, tx, epoch); err != nil {
			return err
		}
		return work(tx)
	})
}

// inPriorityTx runs work in one unit of work without ordinary admission. Only
// the priority path uses it: writes that must succeed whatever the authority
// state, because they make the device safer, give up authority, or record
// safety evidence.
func (s *Service) inPriorityTx(ctx context.Context, work func(*store.Tx) error) error {
	return s.store.InTx(ctx, work)
}

// admitOrdinary requires the owner's runtime lease to be valid and its epoch
// to be neither draining nor killed.
func (s *Service) admitOrdinary(ctx context.Context, tx *store.Tx, epoch string) error {
	if err := tx.Assert(ctx, s.fences.RuntimeOwner, epoch); err != nil {
		return fmt.Errorf("runtime authority: %w", err)
	}
	if err := tx.Assert(ctx, s.fences.EpochControl, epoch); err != nil {
		return fmt.Errorf("epoch control: %w", err)
	}
	return nil
}
