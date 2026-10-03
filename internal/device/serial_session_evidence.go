package device

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// QueryStateEvidence performs one fresh device-state query and packages the
// state with the identity and digest fields required by durable reconciliation.
func (s *DeviceSession) QueryStateEvidence(ctx context.Context) (map[string]any, error) {
	state, err := s.QueryState(ctx)
	if err != nil {
		return nil, err
	}
	digest, err := stateDigest(state)
	if err != nil {
		return nil, fmt.Errorf("digest device state evidence: %w", err)
	}
	return stateFeedbackEvidence(state, digest)
}

func stateFeedbackEvidence(state map[string]any, digest string) (map[string]any, error) {
	feedback := map[string]any{
		"source":         "device.query_state",
		"target":         currentOutputTarget(state),
		"observed_state": state["current_output"],
		"state_digest":   digest,
	}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		return nil, fmt.Errorf("canonicalize device feedback: %w", err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	return packageStateEvidence(state, feedback, digest, feedbackHash)
}

func currentOutputTarget(state map[string]any) string {
	output, _ := state["current_output"].(map[string]any)
	target, _ := output["target"].(string)
	return target
}

func packageStateEvidence(state, feedback map[string]any, digest string, feedbackHash [32]byte) (map[string]any, error) {
	evidence := map[string]any{
		"source":          "device.query_state",
		"evidence_type":   "device_state_feedback",
		"device_id":       stateString(state, "device_id"),
		"boot_id":         stateString(state, "boot_id"),
		"state":           state,
		"state_digest":    digest,
		"feedback":        feedback,
		"feedback_digest": "sha256:" + hex.EncodeToString(feedbackHash[:]),
	}
	return sealStateEvidence(evidence)
}

func sealStateEvidence(evidence map[string]any) (map[string]any, error) {
	withoutDigest := cloneDocument(evidence)
	bundleJSON, err := canonicaljson.Marshal(withoutDigest)
	if err != nil {
		return nil, fmt.Errorf("canonicalize device evidence: %w", err)
	}
	bundleHash := sha256.Sum256(bundleJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(bundleHash[:])
	return evidence, nil
}

func setEvidenceTarget(evidence map[string]any, target string) error {
	evidence["target"] = target
	delete(evidence, "evidence_digest")
	bundleJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("canonicalize device evidence with target: %w", err)
	}
	bundleHash := sha256.Sum256(bundleJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(bundleHash[:])
	return nil
}
