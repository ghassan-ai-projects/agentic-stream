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

// RecordDeviceState records the state a device reported and reports whether
// a reconciliation is required. The first state is clear, the same boot keeps
// its status, and a reboot opens a reconciliation.
func (s *Service) RecordDeviceState(ctx context.Context, owner Owner, document map[string]any) (bool, error) {
	if err := s.checkOwner(owner); err != nil {
		return false, err
	}
	state, err := domain.NewDeviceState(document)
	if err != nil {
		return false, err
	}
	transition, err := s.observeDeviceState(ctx, state, owner, s.now())
	if err != nil {
		return false, fmt.Errorf("record device state: %w", err)
	}
	return transition.Required, nil
}

func (s *Service) observeDeviceState(ctx context.Context, state domain.DeviceState, owner Owner, now time.Time) (domain.StateTransition, error) {
	var transition domain.StateTransition
	err := s.withAdmittedTx(ctx, owner.Epoch, func(tx *sql.Tx) error {
		var err error
		transition, err = recordState(ctx, tx, state, owner, now)
		return err
	})
	return transition, err
}

func recordState(ctx context.Context, tx *sql.Tx, state domain.DeviceState, owner Owner, now time.Time) (domain.StateTransition, error) {
	recorded, err := store.LoadReconciliation(ctx, tx, state.Device.DeviceID)
	if err != nil {
		return domain.StateTransition{}, err
	}
	transition := domain.ObserveState(recorded, state.Device)
	return transition, applyStateTransition(ctx, tx, transition, state, owner, now)
}

func applyStateTransition(ctx context.Context, tx *sql.Tx, transition domain.StateTransition, state domain.DeviceState, owner Owner, now time.Time) error {
	switch transition.Change {
	case domain.StateFirstSeen:
		return store.InsertFirstState(ctx, tx, state, owner, now)
	case domain.StateRefreshed:
		return store.RefreshState(ctx, tx, state, owner, now)
	default:
		if err := store.RecordReboot(ctx, tx, state, owner, now); err != nil {
			return err
		}
		return store.AppendAuthorityEvent(ctx, tx, domain.RebootEvent(transition, state.Device, owner, now))
	}
}

// ReconciliationRequired reports whether ordinary commands to the device are
// blocked by an open reconciliation.
func (s *Service) ReconciliationRequired(ctx context.Context, deviceID string) (bool, error) {
	if deviceID == "" {
		return false, errors.New("device ID is required")
	}
	recorded, err := store.LoadReconciliation(ctx, s.db, deviceID)
	if err != nil {
		return false, err
	}
	return recorded.Required(), nil
}

// OpenReconciliation opens a reconciliation for the current device boot, for
// example when a command may have crossed the gateway but its receipt cannot
// be trusted. A later process observes the same required state.
func (s *Service) OpenReconciliation(ctx context.Context, device DeviceBoot, owner Owner, reason string) error {
	if err := s.checkOpening(device, owner, reason); err != nil {
		return err
	}
	now := s.now()
	err := s.withAdmittedTx(ctx, owner.Epoch, func(tx *sql.Tx) error {
		return openReconciliation(ctx, tx, device, owner, reason, now)
	})
	return wrapOpening(err)
}

// OpenReconciliationAfterAuthorityLoss opens the same reconciliation on the
// priority path, for an unknown outcome whose bytes may have crossed the
// gateway after the owner's lease expired or its epoch was fenced. It only
// makes future commands safer; resolving still needs an admitted owner.
func (s *Service) OpenReconciliationAfterAuthorityLoss(ctx context.Context, device DeviceBoot, owner Owner, reason string) error {
	if err := s.checkOpening(device, owner, reason); err != nil {
		return err
	}
	now := s.now()
	err := s.withPriorityTx(ctx, func(tx *sql.Tx) error {
		return openReconciliation(ctx, tx, device, owner, reason, now)
	})
	return wrapOpening(err)
}

func (s *Service) checkOpening(device DeviceBoot, owner Owner, reason string) error {
	if !device.Complete() || reason == "" {
		return errors.New("device boot and reconciliation reason are required")
	}
	return s.checkOwner(owner)
}

func openReconciliation(ctx context.Context, tx *sql.Tx, device DeviceBoot, owner Owner, reason string, now time.Time) error {
	recorded, err := store.LoadReconciliation(ctx, tx, device.DeviceID)
	if err != nil {
		return err
	}
	alreadyRequired, err := domain.CheckOpenable(recorded, device)
	if err != nil {
		return err
	}
	if !alreadyRequired {
		if err := store.MarkRequired(ctx, tx, device, now); err != nil {
			return err
		}
	}
	return store.AppendAuthorityEvent(ctx, tx, domain.OpeningEvent(device, owner, reason, now))
}

func wrapOpening(err error) error {
	if err != nil {
		return fmt.Errorf("open device reconciliation: %w", err)
	}
	return nil
}
