package domain

import (
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// EvidenceType is the only reconciliation evidence the device authority
// accepts: a device's reported state plus independent feedback about it.
const EvidenceType = "device_state_feedback"

// ReconciliationEvidence is digest-bound evidence about one device boot: the
// state the device reported, independent feedback about it, and a digest over
// the whole bundle. State and feedback are documents owned by their producers;
// the authority binds them by digest without interpreting them.
type ReconciliationEvidence struct {
	Source         string
	Target         string
	Device         DeviceBoot
	State          map[string]any
	StateDigest    string
	Feedback       map[string]any
	FeedbackDigest string
	EvidenceDigest string
}

// evidenceFields is the closed set of fields of an evidence document.
var evidenceFields = []string{
	"boot_id", "device_id", "evidence_digest", "evidence_type", "feedback", "feedback_digest",
	"source", "state", "state_digest", "target",
}

// SealReconciliationEvidence builds evidence for the device boot named by
// state and computes its state, feedback and bundle digests. Target is
// optional; it names the output the evidence is about.
func SealReconciliationEvidence(source, target string, state, feedback map[string]any) (ReconciliationEvidence, error) {
	deviceID, _ := state["device_id"].(string)
	bootID, _ := state["boot_id"].(string)
	evidence := ReconciliationEvidence{Source: source, Target: target, Device: DeviceBoot{DeviceID: deviceID, BootID: bootID}, State: state, Feedback: feedback}
	if source == "" || !evidence.Device.Complete() || feedback == nil {
		return ReconciliationEvidence{}, fmt.Errorf("evidence needs a source, a state naming its device and boot, and feedback")
	}
	var err error
	if evidence.StateDigest, err = documentDigest("reconciliation state", state); err != nil {
		return ReconciliationEvidence{}, err
	}
	if evidence.FeedbackDigest, err = documentDigest("reconciliation feedback", feedback); err != nil {
		return ReconciliationEvidence{}, err
	}
	evidence.EvidenceDigest, err = documentDigest("reconciliation evidence bundle", evidence.bundle())
	return evidence, err
}

// Document is the evidence's wire form.
func (e ReconciliationEvidence) Document() map[string]any {
	document := e.bundle()
	document["evidence_digest"] = e.EvidenceDigest
	return document
}

// bundle is the document the evidence digest covers: everything but itself.
func (e ReconciliationEvidence) bundle() map[string]any {
	bundle := map[string]any{
		"source": e.Source, "evidence_type": EvidenceType,
		"device_id": e.Device.DeviceID, "boot_id": e.Device.BootID,
		"state": e.State, "state_digest": e.StateDigest,
		"feedback": e.Feedback, "feedback_digest": e.FeedbackDigest,
	}
	if e.Target != "" {
		bundle["target"] = e.Target
	}
	return bundle
}

// ParseReconciliationEvidence validates an evidence document for the device
// boot and returns it typed. It checks the envelope, the digest references,
// the device binding, the typed state and feedback, every digest, and finally
// that no unknown field is present. It does not claim that an external
// feedback system actually observed a physical transition.
func ParseReconciliationEvidence(document map[string]any, device DeviceBoot) (ReconciliationEvidence, error) {
	if len(document) == 0 || !device.Complete() {
		return ReconciliationEvidence{}, fmt.Errorf("device reconciliation evidence is required")
	}
	if err := checkEvidenceDocument(document, device); err != nil {
		return ReconciliationEvidence{}, err
	}
	return typedEvidence(document, device), nil
}

// checkEvidenceDocument runs the checks in precedence order: metadata before
// binding, binding before digests, and unknown fields last.
func checkEvidenceDocument(document map[string]any, device DeviceBoot) error {
	if err := checkEvidenceEnvelope(document); err != nil {
		return err
	}
	if err := checkEvidenceDigestReferences(document); err != nil {
		return err
	}
	if err := checkEvidenceDeviceBinding(document, device); err != nil {
		return err
	}
	return checkEvidenceFields(document)
}

func checkEvidenceEnvelope(document map[string]any) error {
	if source, _ := document["source"].(string); source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	if evidenceType, _ := document["evidence_type"].(string); evidenceType != EvidenceType {
		return fmt.Errorf("reconciliation evidence_type must be %s", EvidenceType)
	}
	return nil
}

func checkEvidenceDigestReferences(document map[string]any) error {
	for _, key := range []string{"evidence_digest", "feedback_digest", "state_digest"} {
		reference, _ := document[key].(string)
		if _, err := canonicaljson.DecodeDigest(reference); err != nil {
			return fmt.Errorf("reconciliation %s must be a sha256 reference", key)
		}
	}
	return nil
}

func checkEvidenceDeviceBinding(document map[string]any, device DeviceBoot) error {
	if evidenceDevice, _ := document["device_id"].(string); evidenceDevice != device.DeviceID {
		return fmt.Errorf("reconciliation evidence device identity does not match the binding")
	}
	if evidenceBoot, _ := document["boot_id"].(string); evidenceBoot != device.BootID {
		return fmt.Errorf("reconciliation evidence boot identity does not match the binding")
	}
	if err := checkEvidenceStateBinding(document, device); err != nil {
		return err
	}
	if _, ok := document["feedback"].(map[string]any); !ok {
		return fmt.Errorf("reconciliation evidence must include independent feedback")
	}
	return verifyEvidenceDigests(document)
}

func checkEvidenceStateBinding(document map[string]any, device DeviceBoot) error {
	state, ok := document["state"].(map[string]any)
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

// verifyEvidenceDigests requires the bundle and feedback digests to match the
// document. The state digest is checked against the latest reported state
// when the evidence resolves a reconciliation.
func verifyEvidenceDigests(document map[string]any) error {
	bundle := maps.Clone(document)
	delete(bundle, "evidence_digest")
	if err := matchDigest("evidence_digest", "the evidence bundle", document["evidence_digest"], bundle); err != nil {
		return err
	}
	return matchDigest("feedback_digest", "feedback", document["feedback_digest"], document["feedback"])
}

func matchDigest(field, subject string, provided, value any) error {
	want, err := documentDigest(subject, value)
	if err != nil {
		return err
	}
	if provided != want {
		return fmt.Errorf("reconciliation %s does not match %s", field, subject)
	}
	return nil
}

func checkEvidenceFields(document map[string]any) error {
	for _, field := range slices.Sorted(maps.Keys(document)) {
		if !slices.Contains(evidenceFields, field) {
			return fmt.Errorf("reconciliation evidence has unknown field %q", field)
		}
	}
	return nil
}

func typedEvidence(document map[string]any, device DeviceBoot) ReconciliationEvidence {
	text := func(key string) string { value, _ := document[key].(string); return value }
	state, _ := document["state"].(map[string]any)
	feedback, _ := document["feedback"].(map[string]any)
	return ReconciliationEvidence{
		Source: text("source"), Target: text("target"), Device: device,
		State: state, StateDigest: text("state_digest"),
		Feedback: feedback, FeedbackDigest: text("feedback_digest"), EvidenceDigest: text("evidence_digest"),
	}
}

// documentDigest is the sha256 reference of a document's canonical JSON.
func documentDigest(subject string, document any) (string, error) {
	data, err := canonicaljson.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("canonicalize %s: %w", subject, err)
	}
	return canonicaljson.ContentDigest(data), nil
}
