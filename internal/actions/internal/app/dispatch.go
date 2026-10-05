package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
)

// DispatchOnce processes at most one command. A leased command is marked
// dispatching before the provider call, and all ledger changes are committed
// atomically after the provider returns.
func (s *Service) DispatchOnce(ctx context.Context) (bool, error) {
	leased, found, err := s.leaseNext(ctx)
	if err != nil || !found {
		return found, err
	}
	if err := s.revalidateAuthorization(ctx, leased); err != nil {
		return true, s.finalize(ctx, leased, actionport.Effect{}, fmt.Errorf("authorization revalidation failed: %w", err))
	}
	callCtx, cancel := s.dispatchContext(ctx)
	defer cancel()
	return true, s.dispatchLeased(ctx, callCtx, leased)
}

func (s *Service) dispatchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	callTimeout := s.leaseFor - s.leaseFor/10
	if callTimeout <= 0 {
		callTimeout = s.leaseFor
	}
	return context.WithTimeout(ctx, callTimeout)
}

// dispatchLeased calls the effector with the final authorization check, which
// closes the validation-to-acceptance gap at the effect boundary.
func (s *Service) dispatchLeased(ctx, callCtx context.Context, leased domain.LeasedCommand) error {
	guarded, ok := s.effector.(actionport.AuthorizedEffector)
	if !ok {
		return s.finalize(ctx, leased, actionport.Effect{}, errors.New("configured effector does not enforce dispatch authorization"))
	}
	command := leased.Command
	effect, err := guarded.DispatchAuthorized(callCtx, command, s.store.DispatchAuthorization(command.TenantID, command.NormalizedTarget))
	return s.finalizeProviderDispatch(ctx, callCtx, leased, effect, err)
}

func (s *Service) finalizeProviderDispatch(ctx, callCtx context.Context, leased domain.LeasedCommand, effect actionport.Effect, dispatchErr error) error {
	if errors.Is(dispatchErr, context.DeadlineExceeded) {
		dispatchErr = &actionport.UnknownOutcomeError{Err: dispatchErr}
	}
	check := s.verifyDevice(callCtx, leased, effect, dispatchErr)
	if err := s.finalize(ctx, leased, check.Effect, check.DispatchErr); err != nil {
		return err
	}
	if !check.ReconcilesUnknown() {
		return nil
	}
	return s.reconcileUnknown(ctx, leased.Command.CommandID, check.FinalStatus, check.Evidence)
}

// verifyDevice reads the device state after a successful or unknown dispatch
// when the effector can verify it.
func (s *Service) verifyDevice(ctx context.Context, leased domain.LeasedCommand, effect actionport.Effect, dispatchErr error) domain.DeviceCheck {
	check := domain.DeviceCheck{Effect: effect, DispatchErr: dispatchErr}
	verifier, canVerify := s.effector.(actionport.DeviceStateVerifier)
	if !canVerify || domain.SkipsDeviceVerification(dispatchErr) {
		return check
	}
	check.FinalStatus, check.Evidence, check.VerifyErr = verifier.VerifyDeviceCommand(ctx, leased.Command)
	if check.FinalStatus != "" {
		check.Effect.ObservedEffect = check.Evidence
	}
	if dispatchErr != nil {
		return check
	}
	return domain.ClassifyDeviceVerification(check)
}
