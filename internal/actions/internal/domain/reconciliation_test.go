package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestReconciliationEvidenceValidationOrder(t *testing.T) {
	t.Parallel()
	validDigest := "sha256:" + strings.Repeat("0", 64)
	cases := []struct {
		name        string
		finalStatus string
		evidence    map[string]any
		want        string
	}{
		{"status before evidence", "pending", nil, `invalid reconciliation status "pending"`},
		{"evidence presence", actionport.CommandSucceeded, nil, "reconciliation evidence is required"},
		{"source", actionport.CommandSucceeded, map[string]any{"evidence_type": "provider_observation"}, "reconciliation evidence source is required"},
		{"evidence type", actionport.CommandFailed, map[string]any{"source": "gateway", "evidence_type": "guess"}, "reconciliation evidence_type is required"},
		{"device identity before digest", actionport.CommandSucceeded, map[string]any{"source": "gateway", "evidence_type": "device_state_feedback", "evidence_digest": "invalid"}, "device reconciliation evidence requires device_id"},
		{"first digest cannot be masked", actionport.CommandSucceeded, map[string]any{"source": "gateway", "evidence_type": "provider_observation", "evidence_digest": "invalid", "state_digest": validDigest}, "invalid reconciliation evidence_digest"},
		{"digest required", actionport.CommandManualReview, map[string]any{"source": "gateway", "evidence_type": "provider_observation"}, "reconciliation evidence must include a sha256"},
		{"empty digest falls back", actionport.CommandSucceeded, map[string]any{"source": "gateway", "evidence_type": "provider_observation", "evidence_digest": "", "state_digest": validDigest}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseReconciliation(tc.finalStatus, tc.evidence)
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

func TestReconciledStatusMappings(t *testing.T) {
	t.Parallel()
	cases := []struct{ final, verification, verdict string }{
		{actionport.CommandSucceeded, actionport.VerificationReconciled, "verified"},
		{actionport.CommandFailed, actionport.VerificationRefuted, "refuted"},
		{actionport.CommandManualReview, actionport.VerificationInconclusive, "inconclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.final, func(t *testing.T) {
			t.Parallel()
			if got := ReconciledVerificationStatus(tc.final); got != tc.verification {
				t.Fatalf("verification = %q, want %q", got, tc.verification)
			}
			if got := Verdict(tc.final); got != tc.verdict {
				t.Fatalf("verdict = %q, want %q", got, tc.verdict)
			}
		})
	}
}

func TestReconcilableCommandAcceptsOnlyAwaitingStatuses(t *testing.T) {
	t.Parallel()
	for status, awaiting := range map[string]bool{
		actionport.CommandReconciling: true, actionport.CommandOutcomeUnknown: true, actionport.CommandManualReview: true,
		actionport.CommandSucceeded: false, actionport.CommandFailed: false, actionport.CommandPending: false, actionport.CommandDispatching: false,
	} {
		if err := (ReconcilableCommand{ID: "cmd", Status: status}).RequireAwaitingReconciliation(); (err == nil) != awaiting {
			t.Fatalf("status %q: err = %v, awaiting = %v", status, err, awaiting)
		}
	}
}

func TestReconciledProvenanceRequiresVersionAndFullDigest(t *testing.T) {
	t.Parallel()
	full := make([]byte, 32)
	for name, p := range map[string]ReconciledProvenance{
		"no version": {Version: 0, Digest: full}, "short digest": {Version: 1, Digest: full[:8]},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("%s: incomplete provenance accepted", name)
		}
	}
	if err := (ReconciledProvenance{Version: 2, Digest: full}).Validate(); err != nil {
		t.Fatal(err)
	}
}
