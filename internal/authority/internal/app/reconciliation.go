package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// RecordDeviceState records the state a device reported and reports whether
// a reconciliation is required. The first state is clear, the same boot keeps
// its status, and a reboot opens a reconciliation.
func (s *Service) RecordDeviceState(ctx context.Context, owner domain.Owner, document map[string]any) (bool, error) {
	if err := s.checkOwner(owner); err != nil {
		return false, err
	}
	state, err := domain.NewDeviceState(document)
	if err != nil {
		return false, err
	}
	observation, err := s.observeDeviceState(ctx, state, owner, s.now())
	if err != nil {
		return false, fmt.Errorf("record device state: %w", err)
	}
	return observation.Required, nil
}

func (s *Service) observeDeviceState(ctx context.Context, state domain.DeviceState, owner domain.Owner, now time.Time) (domain.StateObservation, error) {
	var observation domain.StateObservation
	err := s.inAdmittedTx(ctx, owner.Epoch, func(tx *store.Tx) error {
		recorded, err := tx.LoadReconciliation(ctx, state.Device.DeviceID)
		if err != nil {
			return err
		}
		observation = domain.ObserveState(recorded, state, owner)
		return recordObservation(ctx, tx, observation, now)
	})
	return observation, err
}

// recordObservation stores the reported state; a reboot also audits the
// reconciliation it opens.
func recordObservation(ctx context.Context, tx *store.Tx, observation domain.StateObservation, now time.Time) error {
	switch observation.Change {
	case domain.StateFirstSeen:
		return tx.InsertFirstState(ctx, observation.State, observation.Owner, now)
	case domain.StateRefreshed:
		return tx.RefreshState(ctx, observation.State, observation.Owner, now)
	default:
		if err := tx.RecordReboot(ctx, observation.State, observation.Owner, now); err != nil {
			return err
		}
		return tx.AppendAuthorityEvent(ctx, domain.RebootEvent(observation, now))
	}
}

// ReconciliationRequired reports whether ordinary commands to the device are
// blocked by an open reconciliation.
func (s *Service) ReconciliationRequired(ctx context.Context, deviceID string) (bool, error) {
	if deviceID == "" {
		return false, errors.New("device ID is required")
	}
	recorded, err := s.store.LoadReconciliation(ctx, deviceID)
	if err != nil {
		return false, err
	}
	return recorded.Required(), nil
}
