package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

func (s *Service) ResolveReconciliation(ctx context.Context, request domain.ResolutionRequest) (bool, error) {
	if err := request.Check(); err != nil {
		return false, err
	}
	if err := s.checkOwner(request.Owner); err != nil {
		return false, err
	}
	now := s.now()
	err := s.inAdmittedTx(ctx, request.Owner.Epoch, func(tx *store.Tx) error {
		return s.resolveReconciliation(ctx, tx, request, now)
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	return request.Outcome.Clears(), nil
}

func (s *Service) resolveReconciliation(ctx context.Context, tx *store.Tx, request domain.ResolutionRequest, now time.Time) error {
	resolution, err := domain.NewResolution(request)
	if err != nil {
		return err
	}
	if err := s.checkResolvable(ctx, tx, resolution); err != nil {
		return err
	}
	if err := tx.RecordResolution(ctx, resolution, now); err != nil {
		return err
	}
	return tx.AppendAuthorityEvent(ctx, domain.ResolutionEvent(resolution, now))
}

func (s *Service) checkResolvable(ctx context.Context, tx *store.Tx, resolution domain.Resolution) error {
	recorded, err := tx.LoadReconciliation(ctx, resolution.Device.DeviceID)
	if err != nil {
		return err
	}
	if err := domain.CheckResolvable(recorded, resolution); err != nil {
		return err
	}
	return s.checkCommandsReconciled(ctx, tx, resolution.Device)
}

func (s *Service) checkCommandsReconciled(ctx context.Context, tx *store.Tx, device domain.DeviceBoot) error {
	commandIDs, err := tx.ReadBackCommands(ctx, device)
	if err != nil {
		return err
	}
	unresolved, err := tx.CountUnresolved(ctx, s.outcomes, commandIDs)
	if err != nil {
		return err
	}
	return domain.CheckCommandsReconciled(unresolved)
}
