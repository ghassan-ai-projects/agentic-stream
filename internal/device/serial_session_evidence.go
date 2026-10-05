package device

import (
	"context"
	"fmt"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// deviceStateSource names evidence produced by a fresh device-state query.
const deviceStateSource = "device.query_state"

// QueryStateEvidence performs one fresh device-state query about target and
// returns it as sealed reconciliation evidence in its wire form.
func (s *DeviceSession) QueryStateEvidence(ctx context.Context, target string) (map[string]any, error) {
	state, err := s.QueryState(ctx)
	if err != nil {
		return nil, err
	}
	feedback, err := stateFeedback(state)
	if err != nil {
		return nil, err
	}
	evidence, err := deviceauthority.SealReconciliationEvidence(deviceStateSource, target, state, feedback)
	if err != nil {
		return nil, fmt.Errorf("seal device state evidence: %w", err)
	}
	return evidence.Document(), nil
}

// stateFeedback is the device's own observation of its current output.
func stateFeedback(state map[string]any) (map[string]any, error) {
	digest, err := stateDigest(state)
	if err != nil {
		return nil, fmt.Errorf("digest device state evidence: %w", err)
	}
	return map[string]any{
		"source":         deviceStateSource,
		"target":         currentOutputTarget(state),
		"observed_state": state["current_output"],
		"state_digest":   digest,
	}, nil
}

func currentOutputTarget(state map[string]any) string {
	output, _ := state["current_output"].(map[string]any)
	target, _ := output["target"].(string)
	return target
}
