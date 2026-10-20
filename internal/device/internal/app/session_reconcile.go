package app

import (
	"context"
	"fmt"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
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
	persistCtx, cancel := sources.DetachedContext(ctx)
	defer cancel()
	return s.finishReconciliationBarrier(wasRequired, s.authority.RequireReconciliation(persistCtx, s.reconciliationOpening(reason)))
}

func (s *Session) finishReconciliationBarrier(wasRequired bool, barrierErr error) error {
	if barrierErr != nil {
		s.opened = false
		return fmt.Errorf("persist reconciliation barrier: %w", barrierErr)
	}
	if !wasRequired {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return nil
}
