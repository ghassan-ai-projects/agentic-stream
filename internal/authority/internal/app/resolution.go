package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// ResolveReconciliation records evidence and an outcome for the open
// reconciliation of a device boot, and reports whether it cleared. Command
// outcomes must already be reconciled by the dispatcher; manual review keeps
// the reconciliation required.
func (s *Service) ResolveReconciliation(ctx context.Context, device domain.DeviceBoot, owner domain.Owner, outcome domain.ResolutionOutcome, evidence map[string]any) (bool, error) {
	if err := s.checkResolution(device, owner, outcome, evidence); err != nil {
		return false, err
	}
	now := s.now()
	err := s.inAdmittedTx(ctx, owner.Epoch, func(tx *store.Tx) error {
		return s.resolveReconciliation(ctx, tx, device, owner, outcome, evidence, now)
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	return outcome.Clears(), nil
}

func (s *Service) checkResolution(device domain.DeviceBoot, owner domain.Owner, outcome domain.ResolutionOutcome, evidence map[string]any) error {
	if !outcome.Valid() {
		return fmt.Errorf("invalid resolution outcome %q", outcome)
	}
	if !device.Complete() || len(evidence) == 0 {
		return errors.New("device boot and reconciliation evidence are required")
	}
	return s.checkOwner(owner)
}

func (s *Service) resolveReconciliation(ctx context.Context, tx *store.Tx, device domain.DeviceBoot, owner domain.Owner, outcome domain.ResolutionOutcome, evidence map[string]any, now time.Time) error {
	resolution, err := domain.NewResolution(device, owner, outcome, evidence)
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

// checkResolvable requires an open reconciliation bound by the evidence and
// no command of the boot still awaiting reconciliation.
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

// checkCommandsReconciled asks the action ledger about the commands bound to
// the device boot; the ledger owns what "unresolved" means.
func (s *Service) checkCommandsReconciled(ctx context.Context, tx *store.Tx, device domain.DeviceBoot) error {
	commandIDs, err := tx.BoundCommands(ctx, device)
	if err != nil {
		return err
	}
	unresolved, err := tx.CountUnresolved(ctx, s.outcomes, commandIDs)
	if err != nil {
		return err
	}
	return domain.CheckCommandsReconciled(unresolved)
}
