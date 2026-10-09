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
		{"device feedback needs a device id", actionport.CommandSucceeded, deviceEvidenceWithout("device_id"), "device reconciliation evidence requires device_id"},
		{"device feedback needs a boot id", actionport.CommandSucceeded, deviceEvidenceWithout("boot_id"), "device reconciliation evidence requires boot_id"},
		{"device feedback needs the observed state", actionport.CommandSucceeded, deviceEvidenceWithout("state"), "device reconciliation evidence requires state"},
		{"device feedback needs its feedback digest", actionport.CommandSucceeded, deviceEvidenceWithout("feedback_digest"), "device reconciliation evidence requires feedback_digest"},
		{"complete device feedback", actionport.CommandSucceeded, deviceEvidenceWithout(""), ""},
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

func deviceEvidenceWithout(missing string) map[string]any {
	evidence := map[string]any{"source": "gateway", "evidence_type": "device_state_feedback", "device_id": "thermal-01", "boot_id": "boot-A",
		"state": map[string]any{"safe_state": true}, "feedback_digest": "sha256:" + strings.Repeat("0", 64)}
	delete(evidence, missing)
	return evidence
}

func TestAReconciliationOutcomeDocumentCitesItsFinalStatusAndEvidence(t *testing.T) {
	t.Parallel()
	evidence := map[string]any{"source": "gateway"}
	document := ReconciliationOutcomeDocument("cmd-1", "out-2", actionport.CommandFailed, evidence, testNow)
	result, _ := document["result"].(map[string]any)
	if document["status"] != OutcomeReconciled || document["command_id"] != "cmd-1" || document["outcome_id"] != "out-2" ||
		result["final_status"] != actionport.CommandFailed || result["evidence"].(map[string]any)["source"] != "gateway" {
		t.Fatalf("document = %v", document)
	}
	if _, err := OutcomeDigest(document); err != nil {
		t.Fatalf("a reconciliation outcome document is not schema valid: %v", err)
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
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			err := (ReconcilableCommand{ID: "cmd", Status: status}).RequireAwaitingReconciliation()
			if (err == nil) != awaiting || (err != nil && err.Error() != "command cmd is not awaiting reconciliation") {
				t.Fatalf("err = %v, awaiting = %v", err, awaiting)
			}
		})
	}
}

func TestReconciledProvenanceRequiresVersionAndFullDigest(t *testing.T) {
	t.Parallel()
	full := make([]byte, 32)
	for name, p := range map[string]ReconciledProvenance{
		"no version": {Version: 0, Digest: full}, "short digest": {Version: 1, Digest: full[:8]},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(); err == nil {
				t.Fatal("incomplete provenance accepted")
			}
		})
	}
	if err := (ReconciledProvenance{Version: 2, Digest: full}).Validate(); err != nil {
		t.Fatal(err)
	}
}
