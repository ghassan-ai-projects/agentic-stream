package authority

import (
	"strings"
	"testing"
)

func TestReconciliationEvidenceRejectsMetadataBeforeBinding(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("0", 64)
	tests := []struct{ name, key, value, want string }{
		{"source before digest", "source", "", "source is required"},
		{"type before digest", "evidence_type", "other", "must be device_state_feedback"},
		{"digest before device", "evidence_digest", "bad", "must be a sha256 reference"},
		{"device before state", "device_id", "other", "device identity does not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := map[string]any{"source": "feedback", "evidence_type": "device_state_feedback", "evidence_digest": digest, "feedback_digest": digest, "state_digest": digest, "device_id": "device", "boot_id": "boot"}
			evidence[tt.key] = tt.value
			err := ValidateDeviceReconciliationEvidence(evidence, "device", "boot")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation = %v, want %s", err, tt.want)
			}
		})
	}
}
