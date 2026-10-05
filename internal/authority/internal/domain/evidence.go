package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ValidateReconciliationEvidence validates the typed, boot-bound evidence
// envelope. It verifies the document and its digest references; it does not
// claim that an external feedback system actually observed a physical
// transition.
func ValidateReconciliationEvidence(evidence map[string]any, device DeviceBoot) error {
	if len(evidence) == 0 || !device.Complete() {
		return fmt.Errorf("device reconciliation evidence is required")
	}
	if err := checkEvidenceEnvelope(evidence); err != nil {
		return err
	}
	if err := checkEvidenceDigestReferences(evidence); err != nil {
		return err
	}
	return checkEvidenceDeviceBinding(evidence, device)
}

func checkEvidenceEnvelope(evidence map[string]any) error {
	if source, _ := evidence["source"].(string); source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	if evidenceType, _ := evidence["evidence_type"].(string); evidenceType != "device_state_feedback" {
		return fmt.Errorf("reconciliation evidence_type must be device_state_feedback")
	}
	return nil
}

func checkEvidenceDigestReferences(evidence map[string]any) error {
	for _, key := range []string{"evidence_digest", "feedback_digest", "state_digest"} {
		reference, _ := evidence[key].(string)
		if _, err := canonicaljson.DecodeDigest(reference); err != nil {
			return fmt.Errorf("reconciliation %s must be a sha256 reference", key)
		}
	}
	return nil
}

func checkEvidenceDeviceBinding(evidence map[string]any, device DeviceBoot) error {
	if evidenceDevice, _ := evidence["device_id"].(string); evidenceDevice != device.DeviceID {
		return fmt.Errorf("reconciliation evidence device identity does not match the binding")
	}
	if evidenceBoot, _ := evidence["boot_id"].(string); evidenceBoot != device.BootID {
		return fmt.Errorf("reconciliation evidence boot identity does not match the binding")
	}
	if err := checkEvidenceStateBinding(evidence, device); err != nil {
		return err
	}
	feedback, ok := evidence["feedback"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include independent feedback")
	}
	return verifyEvidenceReferences(evidence, feedback)
}

func checkEvidenceStateBinding(evidence map[string]any, device DeviceBoot) error {
	state, ok := evidence["state"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include typed device state")
	}
	if stateDevice, _ := state["device_id"].(string); stateDevice != device.DeviceID {
		return fmt.Errorf("reconciliation state device identity does not match the binding")
	}
	if stateBoot, _ := state["boot_id"].(string); stateBoot != device.BootID {
		return fmt.Errorf("reconciliation state boot identity does not match the binding")
	}
	return nil
}

func verifyEvidenceReferences(evidence, feedback map[string]any) error {
	bundle, err := canonicaljson.Marshal(evidenceWithoutDigest(evidence))
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation evidence bundle: %w", err)
	}
	if provided, _ := evidence["evidence_digest"].(string); provided != canonicaljson.ContentDigest(bundle) {
		return fmt.Errorf("reconciliation evidence_digest does not match the evidence bundle")
	}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation feedback: %w", err)
	}
	if provided, _ := evidence["feedback_digest"].(string); provided != canonicaljson.ContentDigest(feedbackJSON) {
		return fmt.Errorf("reconciliation feedback_digest does not match feedback")
	}
	return nil
}

func evidenceWithoutDigest(evidence map[string]any) map[string]any {
	withoutDigest := make(map[string]any, len(evidence)-1)
	for key, value := range evidence {
		if key != "evidence_digest" {
			withoutDigest[key] = value
		}
	}
	return withoutDigest
}
