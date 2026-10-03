package actions

import (
	"strings"
	"testing"
)

func TestReconciliationEvidenceValidationOrder(t *testing.T) {
	validDigest := "sha256:" + strings.Repeat("0", 64)
	cases := []struct {
		name     string
		evidence map[string]any
		want     string
	}{
		{"device identity before digest", map[string]any{"source": "gateway", "evidence_type": "device_state_feedback", "evidence_digest": "invalid"}, "device reconciliation evidence requires device_id"},
		{"first digest cannot be masked", map[string]any{"source": "gateway", "evidence_type": "provider_observation", "evidence_digest": "invalid", "state_digest": validDigest}, "invalid reconciliation evidence_digest"},
		{"empty digest falls back", map[string]any{"source": "gateway", "evidence_type": "provider_observation", "evidence_digest": "", "state_digest": validDigest}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReconciliationEvidence(tc.evidence)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
