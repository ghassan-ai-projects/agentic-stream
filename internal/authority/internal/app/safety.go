package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// RecordSafeStop records one safe-stop stage on the priority path, so it
// stays available while ordinary authority is draining, expired or fenced.
// Any recorded stage latches the device boot.
func (s *Service) RecordSafeStop(ctx context.Context, claim domain.TargetClaim, stage domain.SafeStopStage, details map[string]any) error {
	if err := s.checkClaim(claim); err != nil {
		return err
	}
	if !stage.Valid() {
		return fmt.Errorf("invalid safe-stop stage %q", stage)
	}
	event := domain.SafeStopEvent(claim, stage, details, s.now())
	err := s.inPriorityTx(ctx, func(tx *store.Tx) error {
		return tx.AppendAuthorityEvent(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("record safe stop: %w", err)
	}
	return nil
}

// SafeStopLatched reports whether a safe stop was recorded for the device
// boot. There is no clear operation: a new owner stays stopped until the
// external safety owner has handled the physical condition and the device
// reboots.
func (s *Service) SafeStopLatched(ctx context.Context, device domain.DeviceBoot) (bool, error) {
	if !device.Complete() {
		return false, errors.New("device boot is required")
	}
	return s.store.SafeStopLatched(ctx, device)
}

// RecordSafetyEvent appends one piece of validated safety evidence. Evidence
// is recorded on the priority path whatever the authority state.
func (s *Service) RecordSafetyEvent(ctx context.Context, event domain.SafetyEvent) error {
	prepared, err := domain.PrepareSafetyEvent(event, s.now())
	if err != nil {
		return err
	}
	err = s.inPriorityTx(ctx, func(tx *store.Tx) error {
		return tx.AppendSafetyEvent(ctx, prepared)
	})
	if err != nil {
		return fmt.Errorf("record safety event: %w", err)
	}
	return nil
}
