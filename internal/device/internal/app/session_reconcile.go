package app

import (
	"context"
	"errors"
	"fmt"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

// ResolveReconciliation records typed state/feedback evidence for the device
// barrier. Unknown command outcomes must already have gone through the
// dispatcher reconciliation path; the durable store refuses to clear while
// command ledgers remain unresolved. Manual review leaves the barrier open.
func (s *Session) ResolveReconciliation(ctx context.Context, finalStatus string, evidence map[string]any) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return false, fmt.Errorf("device session is not open")
	}
	if !s.reconciliationRequired {
		return false, fmt.Errorf("device reconciliation barrier is not open")
	}
	return s.resolveCurrentState(ctx, finalStatus, evidence)
}

func (s *Session) resolveCurrentState(ctx context.Context, finalStatus string, evidence map[string]any) (bool, error) {
	if s.stateQueryRequired {
		return false, fmt.Errorf("device state query is required before reconciliation can be resolved")
	}
	if evidence == nil || evidence["state_digest"] != s.stateDigest {
		return false, fmt.Errorf("reconciliation evidence must bind the latest device state digest")
	}
	if err := s.authority.AssertRuntime(ctx, s.ownerEpoch); err != nil {
		return false, fmt.Errorf("assert reconciliation authority: %w", err)
	}
	return s.persistResolvedState(ctx, finalStatus, evidence)
}

func (s *Session) persistResolvedState(ctx context.Context, finalStatus string, evidence map[string]any) (bool, error) {
	cleared, err := s.authority.ResolveReconciliation(ctx, deviceauthority.ResolutionRequest{
		Device: s.deviceBoot(), Owner: s.owner(), Outcome: deviceauthority.ResolutionOutcome(finalStatus), Evidence: evidence,
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	if cleared {
		s.reconciliationRequired = false
	}
	return cleared, nil
}

func (s *Session) requireReconciliation(ctx context.Context, reason string) error {
	wasRequired := s.reconciliationRequired
	s.stateQueryRequired = true
	s.reconciliationRequired = true
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reconciliationPersistTimeout)
	defer cancel()
	return s.finishReconciliationBarrier(persistCtx, reason, wasRequired, s.openReconciliation(persistCtx, reason))
}

// openReconciliation persists the reconciliation for the current boot.
func (s *Session) openReconciliation(ctx context.Context, reason string) error {
	return s.authority.OpenReconciliation(ctx, s.reconciliationOpening(reason)) //nolint:wrapcheck // finishReconciliationBarrier wraps it.
}

func (s *Session) finishReconciliationBarrier(persistCtx context.Context, reason string, wasRequired bool, barrierErr error) error {
	barrierErr = s.recoverReconciliationBarrier(persistCtx, reason, barrierErr)
	if barrierErr != nil {
		s.opened = false
		return fmt.Errorf("persist reconciliation barrier: %w", barrierErr)
	}
	if !wasRequired {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return nil
}

func (s *Session) recoverReconciliationBarrier(persistCtx context.Context, reason string, barrierErr error) error {
	if isAuthorityFailure(barrierErr) {
		recoveryErr := s.authority.OpenReconciliationAfterAuthorityLoss(persistCtx, s.reconciliationOpening(reason))
		if recoveryErr == nil {
			barrierErr = nil
		} else {
			barrierErr = errors.Join(barrierErr, recoveryErr)
		}
	}
	return barrierErr
}

func isAuthorityFailure(err error) bool {
	return errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) || errors.Is(err, runtimecontrol.ErrEpochKilled) || errors.Is(err, runtimecontrol.ErrEpochDraining)
}
