package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func TestAuthorizationAcceptsCurrentApprovedRecords(t *testing.T) {
	t.Parallel()
	for _, risk := range []string{"R1", "R2"} {
		records := authorizationRecords(t, risk)
		if _, err := records.VerifiedCommand(); err != nil {
			t.Fatalf("%s command: %v", risk, err)
		}
		for name, err := range map[string]error{
			"approved": records.RequireApprovedIntent(), "approval": records.CheckApproval(testNow), "current": records.RequireCurrent(),
			"intent": records.CheckIntent(testNow), "decision": records.CheckDecision(),
		} {
			if err != nil {
				t.Fatalf("%s %s: %v", risk, name, err)
			}
		}
	}
}

func TestAuthorizationRefusesEachStaleOrAlteredRecord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*AuthorizationRecords)
		check  func(AuthorizationRecords) error
		want   string
	}{
		{"altered command document", func(r *AuthorizationRecords) { r.Command.Target = "motor/2" }, func(r AuthorizationRecords) error { _, err := r.VerifiedCommand(); return err }, "command ledger identity mismatch"},
		{"command digest", func(r *AuthorizationRecords) { r.Command.SHA = make([]byte, 32) }, func(r AuthorizationRecords) error { _, err := r.VerifiedCommand(); return err }, "command ledger identity mismatch"},
		{"policy not approved", func(r *AuthorizationRecords) { r.Intent.PolicyStatus = "pending" }, AuthorizationRecords.RequireApprovedIntent, "command is no longer approved for its intent"},
		{"decision not accepted", func(r *AuthorizationRecords) { r.Decision.ValidationStatus = "rejected" }, AuthorizationRecords.RequireApprovedIntent, "command is no longer approved for its intent"},
		{"route differs from intent type", func(r *AuthorizationRecords) { r.Command.Route = "other" }, AuthorizationRecords.RequireApprovedIntent, "command is no longer approved for its intent"},
		{"newer material Situation version", func(r *AuthorizationRecords) { r.Situation.LastMaterialVersion = 2 }, AuthorizationRecords.RequireCurrent, "command authorization is stale"},
		{"episode still running", func(r *AuthorizationRecords) { r.Episode.ProducedDecision = false }, AuthorizationRecords.RequireCurrent, "command authorization is stale"},
		{"episode tenant", func(r *AuthorizationRecords) { r.Episode.TenantID = "other" }, AuthorizationRecords.RequireCurrent, "command authorization is stale"},
		{"decision version", func(r *AuthorizationRecords) { r.Decision.SituationVersion = 2 }, AuthorizationRecords.RequireCurrent, "command authorization is stale"},
		{"intent digest", func(r *AuthorizationRecords) { r.Intent.SHA = make([]byte, 32) }, func(r AuthorizationRecords) error { return r.CheckIntent(testNow) }, "intent authorization is invalid"},
		{"intent identity", func(r *AuthorizationRecords) { r.Intent.Risk = "R0" }, func(r AuthorizationRecords) error { return r.CheckIntent(testNow) }, "intent authorization identity mismatch"},
		{"decision digest", func(r *AuthorizationRecords) { r.Decision.SHA = make([]byte, 32) }, AuthorizationRecords.CheckDecision, "decision authorization is invalid"},
		{"decision episode", func(r *AuthorizationRecords) { r.Decision.EpisodeID = "epi-2" }, AuthorizationRecords.CheckDecision, "decision authorization identity mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			records := authorizationRecords(t, "R1")
			tc.mutate(&records)
			if err := tc.check(records); err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestIntentExpiresAtItsDeadline(t *testing.T) {
	t.Parallel()
	records := authorizationRecords(t, "R1")
	if err := records.CheckIntent(testNow.Add(2 * time.Hour)); err == nil || err.Error() != "intent authorization is expired" {
		t.Fatalf("expired intent err = %v", err)
	}
	records.Intent.ExpiresAt = time.Time{}
	if err := records.CheckIntent(testNow); err == nil || err.Error() != "intent authorization is expired" {
		t.Fatalf("zero expiry err = %v", err)
	}
}

func TestApprovalIsCheckedExactlyWhereThePolicyRoutesToApproval(t *testing.T) {
	t.Parallel()
	for _, risk := range contractstest.RiskClasses() {
		for _, flagged := range []bool{false, true} {
			records := authorizationRecords(t, string(risk))
			records.Intent.RequiresApproval = flagged
			records.Approval = ApprovalRow{}
			needsApproval := contractsv1.RouteFor(contractsv1.RiskClass(risk), flagged) == contractsv1.RouteApproval
			err := records.CheckApproval(testNow)
			if needsApproval != (err != nil) || err != nil && err.Error() != "approved intent has no approved approval record" {
				t.Fatalf("%s requires_approval=%t: err = %v", risk, flagged, err)
			}
		}
	}
}

func TestApprovalRequiredIntentsNeedAnUnexpiredApproval(t *testing.T) {
	t.Parallel()
	for _, risk := range []string{"R1", "R2"} {
		records := authorizationRecords(t, risk)
		records.Intent.RequiresApproval = true
		if err := records.CheckApproval(testNow); err != nil {
			t.Fatalf("%s current approval: %v", risk, err)
		}
		if err := records.CheckApproval(testNow.Add(2 * time.Hour)); err == nil || err.Error() != "approval is expired" {
			t.Fatalf("%s expired approval err = %v", risk, err)
		}
	}
}

func TestPolicyDigestMustMatchLatestApproval(t *testing.T) {
	t.Parallel()
	if err := CheckPolicyDigest("sha256:a", "sha256:a"); err != nil {
		t.Fatal(err)
	}
	if err := CheckPolicyDigest("sha256:a", "sha256:b"); err == nil || err.Error() != "command policy digest is stale" {
		t.Fatalf("err = %v", err)
	}
}

func TestDocumentAccessorsTolerateMissingFields(t *testing.T) {
	t.Parallel()
	d := Document{"s": "x", "i": float64(3), "o": map[string]any{"k": 1}, "bad": "sha256:zz"}
	if d.String("s") != "x" || d.String("missing") != "" || d.Int("i") != 3 || d.Int64("i") != 3 || d.Int("s") != 0 {
		t.Fatal("scalar accessors changed")
	}
	if d.Object("o")["k"] != 1 || d.Object("s") != nil || d.Digest("bad") != nil || d.Digest("missing") != nil {
		t.Fatal("object and digest accessors changed")
	}
}

// A newer Situation version that cognition did not judge material keeps the
// command's authority current (ADR-018).
func TestAuthorizationKeepsCommandsCurrentAcrossVersionsThatAreNotMaterial(t *testing.T) {
	t.Parallel()
	records := authorizationRecords(t, "R1")
	records.Situation.LastMaterialVersion = 0
	if err := records.RequireCurrent(); err != nil {
		t.Fatalf("command refused although no material version followed its intent: %v", err)
	}
}
