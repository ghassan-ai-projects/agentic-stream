package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// deviceStateSource names evidence produced by a fresh device-state query.
const deviceStateSource = "device.query_state"

// QueryStateEvidence performs one fresh device-state query about target and
// returns it as sealed reconciliation evidence in its wire form.
func (s *Session) QueryStateEvidence(ctx context.Context, target string) (domain.State, map[string]any, error) {
	state, err := s.QueryState(ctx)
	if err != nil {
		return domain.State{}, nil, err
	}
	feedback, err := stateFeedback(state)
	if err != nil {
		return domain.State{}, nil, err
	}
	evidence, err := deviceauthority.SealReconciliationEvidence(deviceStateSource, target, state.Document, feedback)
	if err != nil {
		return domain.State{}, nil, fmt.Errorf("seal device state evidence: %w", err)
	}
	return state, evidence.Document(), nil
}

// stateFeedback is the device's own observation of its current output.
func stateFeedback(state domain.State) (map[string]any, error) {
	digest, err := state.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest device state evidence: %w", err)
	}
	return map[string]any{
		"source":         deviceStateSource,
		"target":         state.Output.Target,
		"observed_state": state.Document["current_output"],
		"state_digest":   digest,
	}, nil
}
