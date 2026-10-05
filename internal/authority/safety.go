package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// RecordSafeStop records one safe-stop stage on the priority path, so it
// stays available while ordinary authority is draining, expired or fenced.
// Any recorded stage latches the device boot.
func (s *Service) RecordSafeStop(ctx context.Context, claim TargetClaim, stage SafeStopStage, details map[string]any) error {
	if err := s.checkClaim(claim); err != nil {
		return err
	}
	if !stage.Valid() {
		return fmt.Errorf("invalid safe-stop stage %q", stage)
	}
	event := domain.SafeStopEvent(claim, stage, details, s.now())
	err := s.withPriorityTx(ctx, func(tx *sql.Tx) error {
		return store.AppendAuthorityEvent(ctx, tx, event)
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
func (s *Service) SafeStopLatched(ctx context.Context, device DeviceBoot) (bool, error) {
	if !device.Complete() {
		return false, errors.New("device boot is required")
	}
	return store.SafeStopLatched(ctx, s.db, device)
}

// RecordSafetyEvent appends one piece of validated safety evidence. Evidence
// is recorded on the priority path whatever the authority state.
func (s *Service) RecordSafetyEvent(ctx context.Context, event SafetyEvent) error {
	prepared, err := domain.PrepareSafetyEvent(event, s.now())
	if err != nil {
		return err
	}
	err = s.withPriorityTx(ctx, func(tx *sql.Tx) error {
		return store.AppendSafetyEvent(ctx, tx, prepared)
	})
	if err != nil {
		return fmt.Errorf("record safety event: %w", err)
	}
	return nil
}
