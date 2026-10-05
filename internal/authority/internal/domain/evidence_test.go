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

func TestValidateReconciliationEvidence(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evidence := cloneEvidence(valid)
			tt.mutate(evidence)
			err := ValidateReconciliationEvidence(evidence, bootOne)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("validation = %v, want %q", err, tt.want)
			}
		})
	}
	if err := ValidateReconciliationEvidence(nil, bootOne); err == nil {
		t.Fatal("empty evidence was accepted")
	}
}

func rehash(evidence map[string]any) string {
	data, err := canonicaljson.Marshal(evidenceWithoutDigest(evidence))
	if err != nil {
		panic(err)
	}
	return canonicaljson.ContentDigest(data)
}
