package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ValidateDeviceReconciliationEvidence validates the typed, boot-bound
// evidence envelope used by the serial boundary and dispatcher. It verifies
// the supplied document references; it does not claim that an external
// feedback system actually observed a physical transition.
func ValidateDeviceReconciliationEvidence(evidence map[string]any, deviceID, bootID string) error {
	if len(evidence) == 0 || deviceID == "" || bootID == "" {
		return fmt.Errorf("device reconciliation evidence is required")
	}
	source, _ := evidence["source"].(string)
	if source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	evidenceType, _ := evidence["evidence_type"].(string)
	if evidenceType != "device_state_feedback" {
		return fmt.Errorf("reconciliation evidence_type must be device_state_feedback")
	}
	for _, key := range []string{"evidence_digest", "feedback_digest", "state_digest"} {
		digest, _ := evidence[key].(string)
		if !validSHA256Reference(digest) {
			return fmt.Errorf("reconciliation %s must be a sha256 reference", key)
		}
	}
	if evidenceDevice, _ := evidence["device_id"].(string); evidenceDevice != deviceID {
		return fmt.Errorf("reconciliation evidence device identity does not match the binding")
	}
	if evidenceBoot, _ := evidence["boot_id"].(string); evidenceBoot != bootID {
		return fmt.Errorf("reconciliation evidence boot identity does not match the binding")
	}
	state, ok := evidence["state"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include typed device state")
	}
	if stateDevice, _ := state["device_id"].(string); stateDevice != deviceID {
		return fmt.Errorf("reconciliation state device identity does not match the binding")
	}
	if stateBoot, _ := state["boot_id"].(string); stateBoot != bootID {
		return fmt.Errorf("reconciliation state boot identity does not match the binding")
	}
	feedback, ok := evidence["feedback"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include independent feedback")
	}
	return verifyEvidenceReferences(evidence, feedback)
}

func verifyEvidenceReferences(evidence map[string]any, feedback map[string]any) error {
	withoutDigest := make(map[string]any, len(evidence)-1)
	for key, value := range evidence {
		if key != "evidence_digest" {
			withoutDigest[key] = value
		}
	}
	bundle, err := canonicaljson.Marshal(withoutDigest)
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation evidence bundle: %w", err)
	}
	bundleHash := sha256.Sum256(bundle)
	provided, _ := evidence["evidence_digest"].(string)
	if provided != "sha256:"+hex.EncodeToString(bundleHash[:]) {
		return fmt.Errorf("reconciliation evidence_digest does not match the evidence bundle")
	}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation feedback: %w", err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	provided, _ = evidence["feedback_digest"].(string)
	if provided != "sha256:"+hex.EncodeToString(feedbackHash[:]) {
		return fmt.Errorf("reconciliation feedback_digest does not match feedback")
	}
	return nil
}
