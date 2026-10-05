package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// OpenReconciliation opens a reconciliation for the current device boot, for
// example when a command may have crossed the gateway but its receipt cannot
// be trusted. A later process observes the same required state.
func (s *Service) OpenReconciliation(ctx context.Context, opening domain.ReconciliationOpening) error {
	if err := s.checkOpening(opening); err != nil {
		return err
	}
	now := s.now()
	err := s.inAdmittedTx(ctx, opening.Owner.Epoch, func(tx *store.Tx) error {
		return openReconciliation(ctx, tx, opening, now)
	})
	return wrapOpening(err)
}

// OpenReconciliationAfterAuthorityLoss opens the same reconciliation on the
// priority path, for an unknown outcome whose bytes may have crossed the
// gateway after the owner's lease expired or its epoch was fenced. It only
// makes future commands safer; resolving still needs an admitted owner.
func (s *Service) OpenReconciliationAfterAuthorityLoss(ctx context.Context, opening domain.ReconciliationOpening) error {
	if err := s.checkOpening(opening); err != nil {
		return err
	}
	now := s.now()
	err := s.inPriorityTx(ctx, func(tx *store.Tx) error {
		return openReconciliation(ctx, tx, opening, now)
	})
	return wrapOpening(err)
}

func (s *Service) checkOpening(opening domain.ReconciliationOpening) error {
	if !opening.Complete() {
		return errors.New("device boot, owner and reconciliation reason are required")
	}
	return s.checkOwner(opening.Owner)
}

func openReconciliation(ctx context.Context, tx *store.Tx, opening domain.ReconciliationOpening, now time.Time) error {
	recorded, err := tx.LoadReconciliation(ctx, opening.Device.DeviceID)
	if err != nil {
		return err
	}
	alreadyRequired, err := domain.CheckOpenable(recorded, opening.Device)
	if err != nil {
		return err
	}
	if !alreadyRequired {
		if err := tx.MarkRequired(ctx, opening.Device, now); err != nil {
			return err
		}
	}
	return tx.AppendAuthorityEvent(ctx, domain.OpeningEvent(opening, now))
}

func wrapOpening(err error) error {
	if err != nil {
		return fmt.Errorf("open device reconciliation: %w", err)
	}
	return nil
}
