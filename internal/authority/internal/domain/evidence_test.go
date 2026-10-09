package domain

import (
	"maps"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// validEvidence builds a digest-consistent evidence envelope for device.
func validEvidence(t *testing.T, device DeviceBoot, target string) map[string]any {
	t.Helper()
	state := map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID, "safe_state": true}
	feedback := map[string]any{"target": target, "observed_state": "safe"}
	evidence := map[string]any{
		"device_id": device.DeviceID, "boot_id": device.BootID, "target": target,
		"evidence_type": "device_state_feedback", "source": "independent-feedback",
		"state": state, "state_digest": mustDigest(t, state),
		"feedback": feedback, "feedback_digest": mustDigest(t, feedback),
	}
	evidence["evidence_digest"] = mustDigest(t, evidence)
	return evidence
}

func mustDigest(t *testing.T, value any) string {
	t.Helper()
	data, err := canonicaljson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return canonicaljson.ContentDigest(data)
}

func cloneEvidence(evidence map[string]any) map[string]any {
	return maps.Clone(evidence)
}

func TestReconciliationEvidenceIsValidatedInADeclaredOrder(t *testing.T) {
	t.Parallel()
	valid := validEvidence(t, bootOne, "fan-01")
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"valid", func(map[string]any) {}, ""},
		{"source before digest", func(e map[string]any) { e["source"] = ""; e["evidence_digest"] = "bad" }, "source is required"},
		{"type before digest", func(e map[string]any) { e["evidence_type"] = "other"; e["evidence_digest"] = "bad" }, "must be device_state_feedback"},
		{"digest reference before device", func(e map[string]any) { e["state_digest"] = "bad"; e["device_id"] = "other" }, "state_digest must be a sha256 reference"},
		{"device before state", func(e map[string]any) { e["device_id"] = "other"; delete(e, "state") }, "evidence device identity does not match"},
		{"boot", func(e map[string]any) { e["boot_id"] = "other" }, "evidence boot identity does not match"},
		{"missing state", func(e map[string]any) { delete(e, "state") }, "must include typed device state"},
		{"state for another boot", func(e map[string]any) { e["state"] = map[string]any{"device_id": "thermal-01", "boot_id": "x"} }, "state boot identity"},
		{"state for another device", func(e map[string]any) { e["state"] = map[string]any{"device_id": "x"} }, "state device identity"},
		{"missing feedback", func(e map[string]any) { delete(e, "feedback") }, "must include independent feedback"},
		{"tampered bundle", func(e map[string]any) { e["source"] = "someone-else" }, "evidence_digest does not match"},
		{"tampered feedback", func(e map[string]any) { e["feedback_digest"] = digestOther; e["evidence_digest"] = rehash(e) }, "feedback_digest does not match"},
		{"unknown field", func(e map[string]any) { e["note"] = "x"; e["evidence_digest"] = rehash(e) }, `unknown field "note"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evidence := cloneEvidence(valid)
			tt.mutate(evidence)
			_, err := ParseReconciliationEvidence(evidence, bootOne)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("validation = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := ParseReconciliationEvidence(nil, bootOne); err == nil {
		t.Fatal("empty evidence was accepted")
	}
}

func TestSealedEvidenceParsesBackToItself(t *testing.T) {
	t.Parallel()
	state := map[string]any{"device_id": bootOne.DeviceID, "boot_id": bootOne.BootID, "safe_state": true}
	feedback := map[string]any{"target": "fan-01", "observed_state": "safe"}
	for _, target := range []string{"", "fan-01"} {
		sealed, err := SealReconciliationEvidence("device.query_state", target, state, feedback)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseReconciliationEvidence(sealed.Document(), bootOne)
		if err != nil {
			t.Fatalf("target %q: sealed evidence did not parse: %v", target, err)
		}
		if parsed.EvidenceDigest != sealed.EvidenceDigest || parsed.Target != target || parsed.Device != bootOne {
			t.Fatalf("parsed = %+v, sealed = %+v", parsed, sealed)
		}
	}
	if sealed, err := SealReconciliationEvidence("independent-feedback", "fan-01", state, feedback); err != nil || sealed.Document()["evidence_digest"] != validEvidence(t, bootOne, "fan-01")["evidence_digest"] {
		t.Fatalf("sealing disagrees with the reference digest scheme: %v", err)
	}
	if _, err := SealReconciliationEvidence("", "", state, feedback); err == nil {
		t.Fatal("evidence without a source was sealed")
	}
	if _, err := SealReconciliationEvidence("s", "", map[string]any{"device_id": "d"}, feedback); err == nil {
		t.Fatal("evidence for a state without a boot was sealed")
	}
}

func rehash(evidence map[string]any) string {
	bundle := maps.Clone(evidence)
	delete(bundle, "evidence_digest")
	data, err := canonicaljson.Marshal(bundle)
	if err != nil {
		panic(err)
	}
	return canonicaljson.ContentDigest(data)
}
