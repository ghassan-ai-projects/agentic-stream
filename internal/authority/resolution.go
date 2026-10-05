package authority

import (
	"context"
	"database/sql"
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
func (s *Service) ResolveReconciliation(ctx context.Context, device DeviceBoot, owner Owner, outcome ResolutionOutcome, evidence map[string]any) (bool, error) {
	if err := s.checkResolution(device, owner, outcome, evidence); err != nil {
		return false, err
	}
	now := s.now()
	err := s.withAdmittedTx(ctx, owner.Epoch, func(tx *sql.Tx) error {
		return resolveReconciliation(ctx, tx, device, owner, outcome, evidence, now)
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	return outcome.Clears(), nil
}

func (s *Service) checkResolution(device DeviceBoot, owner Owner, outcome ResolutionOutcome, evidence map[string]any) error {
	if !outcome.Valid() {
		return fmt.Errorf("invalid resolution outcome %q", outcome)
	}
	if !device.Complete() || len(evidence) == 0 {
		return errors.New("device boot and reconciliation evidence are required")
	}
	return s.checkOwner(owner)
}

func resolveReconciliation(ctx context.Context, tx *sql.Tx, device DeviceBoot, owner Owner, outcome ResolutionOutcome, evidence map[string]any, now time.Time) error {
	resolution, err := domain.NewResolution(device, owner, outcome, evidence)
	if err != nil {
		return err
	}
	if err := checkResolvable(ctx, tx, resolution); err != nil {
		return err
	}
	if err := store.RecordResolution(ctx, tx, resolution, now); err != nil {
		return err
	}
	return store.AppendAuthorityEvent(ctx, tx, domain.ResolutionEvent(resolution, now))
}

// checkResolvable requires an open reconciliation bound by the evidence and
// no command of the boot still awaiting reconciliation.
func checkResolvable(ctx context.Context, tx *sql.Tx, resolution domain.Resolution) error {
	recorded, err := store.LoadReconciliation(ctx, tx, resolution.Device.DeviceID)
	if err != nil {
		return err
	}
	if err := domain.CheckResolvable(recorded, resolution); err != nil {
		return err
	}
	unresolved, err := store.CountUnresolvedCommands(ctx, tx, resolution.Device)
	if err != nil {
		return err
	}
	return domain.CheckCommandsReconciled(unresolved)
}
