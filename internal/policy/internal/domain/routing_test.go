package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

// The risk table is owned by contractsv1 (DUP-001); this is the one table of
// the route policy decides for every risk class and catalog approval flag.
func TestRiskRouteForEveryRiskClassAndCatalogApproval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		risk             string
		requiresApproval int
		wantRoute        string
		wantReason       string
	}{
		{"R0", 0, "automatic", ""},
		{"R0", 1, "approval", ""},
		{"R1", 0, "automatic", ""},
		{"R1", 1, "approval", ""},
		{"R2", 0, "approval", ""},
		{"R2", 1, "approval", ""},
		{"R3", 0, "denied", "risk_policy_denied"},
		{"R3", 1, "denied", "risk_policy_denied"},
		{"R4", 0, "denied", "risk_policy_denied"},
		{"R4", 1, "denied", "risk_policy_denied"},
		{"R5", 0, "denied", "unknown_risk_class"},
		{"", 1, "denied", "unknown_risk_class"},
		{"r1", 0, "denied", "unknown_risk_class"},
	}
	for _, test := range tests {
		route, reason := RiskRoute(IntentRecord{RiskClass: test.risk, RequiresApproval: test.requiresApproval})
		if route != test.wantRoute || reason != test.wantReason {
			t.Errorf("RiskRoute(%q, requires_approval=%d) = %s/%q, want %s/%q", test.risk, test.requiresApproval, route, reason, test.wantRoute, test.wantReason)
		}
	}
}

func TestThePolicyDocumentIsDerivedFromTheSameRiskTable(t *testing.T) {
	t.Parallel()
	document := CanonicalDocumentForVersion("v1")
	riskPolicy := document["risk_policy"].(map[string]any)
	health := document["incomplete_source_health"].(map[string]any)
	for _, risk := range contractstest.RiskClasses() {
		for _, approval := range []int{0, 1} {
			route, _ := RiskRoute(IntentRecord{RiskClass: string(risk), RequiresApproval: approval})
			if want := contractsv1.RouteFor(contractsv1.RiskClass(risk), approval != 0); route != string(want) {
				t.Errorf("%s requires_approval=%d: route %s, want %s", risk, approval, route, want)
			}
		}
		if got := riskPolicy[string(risk)]; got != string(contractsv1.RouteFor(contractsv1.RiskClass(risk), false)) {
			t.Errorf("%s: the policy document routes it %v", risk, got)
		}
		_, documented := health[string(risk)]
		incomplete := SourceHealthIncomplete(IntentRecord{RiskClass: string(risk), CurrentCompleteness: "uncertain"})
		if documented != incomplete {
			t.Errorf("%s: document incomplete=%t, rule=%t", risk, documented, incomplete)
		}
	}
}

func TestFreshnessFailureChecksLifecycleThenVersionThenHealthThenExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	base := IntentRecord{EpisodeProducedDecision: true, SituationVersion: 1, LastMaterialVersion: 1, RiskClass: "R2", CurrentCompleteness: "on_time", ExpiresAt: now.Add(time.Hour)}
	tests := []struct {
		name       string
		change     func(*IntentRecord)
		wantStatus string
		wantReason string
	}{
		{"a fresh intent has no failure", func(*IntentRecord) {}, "", ""},
		{"lifecycle comes first", func(r *IntentRecord) {
			r.EpisodeProducedDecision = false
			r.LastMaterialVersion = 2
			r.ExpiresAt = time.Time{}
		}, "denied", "episode_not_concluded"},
		{"a newer version that is not material stays fresh", func(r *IntentRecord) { r.CurrentSituation, r.LastMaterialVersion = 3, 1 }, "", ""},
		{"a newer material version is stale before health is read", func(r *IntentRecord) {
			r.LastMaterialVersion = 2
			r.CurrentCompleteness = "uncertain"
		}, "stale", "situation_version_stale"},
		{"health comes before expiry", func(r *IntentRecord) { r.CurrentCompleteness = "uncertain"; r.ExpiresAt = time.Time{} }, "denied", "source_health_incomplete"},
		{"incomplete health does not stop a low-risk intent", func(r *IntentRecord) { r.RiskClass, r.CurrentCompleteness = "R1", "uncertain" }, "", ""},
		{"an unreadable expiry denies rather than expires", func(r *IntentRecord) { r.ExpiresAt, r.ExpiryUnreadable = time.Time{}, true }, "denied", "intent_expiry_unreadable"},
		{"expiry is inclusive of the evaluation instant", func(r *IntentRecord) { r.ExpiresAt = now }, "expired", "intent_expired"},
		{"one nanosecond before expiry is live", func(r *IntentRecord) { r.ExpiresAt = now.Add(time.Nanosecond) }, "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			row := base
			test.change(&row)
			status, reason, expires := FreshnessFailure(row, now)
			if status != test.wantStatus || reason != test.wantReason {
				t.Fatalf("FreshnessFailure = %q/%q, want %q/%q", status, reason, test.wantStatus, test.wantReason)
			}
			if reason == "" && !expires.Equal(row.ExpiresAt) {
				t.Fatalf("a fresh intent reported expiry %v, want %v", expires, row.ExpiresAt)
			}
		})
	}
}

func TestApprovalDispositionOrdersResolvedThenStaleThenExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	fresh := IntentRecord{SituationVersion: 1, LastMaterialVersion: 1}
	superseded := IntentRecord{SituationVersion: 1, LastMaterialVersion: 2}
	pending := ApprovalRecord{Status: "pending", ExpiresAt: now.Add(time.Hour)}
	tests := []struct {
		name     string
		row      IntentRecord
		approval ApprovalRecord
		approved bool
		want     string
	}{
		{"a live approval of a fresh intent is authorized", fresh, pending, true, "authorize"},
		{"a live denial of a fresh intent is authorized", fresh, pending, false, "authorize"},
		{"an approval that expires now is expired", fresh, ApprovalRecord{Status: "pending", ExpiresAt: now}, true, "expired"},
		{"a denial after the deadline is expired", fresh, ApprovalRecord{Status: "pending", ExpiresAt: now}, false, "expired"},
		{"an approval of a superseded intent is stale", superseded, pending, true, "stale"},
		{"a denial of a superseded intent is still authorized", superseded, pending, false, "authorize"},
		{"staleness outranks expiry", superseded, ApprovalRecord{Status: "pending", ExpiresAt: now}, true, "stale"},
		{"an approved approval is resolved even when superseded", superseded, ApprovalRecord{Status: "approved"}, true, "resolved"},
		{"a denied approval is resolved", fresh, ApprovalRecord{Status: "denied", ExpiresAt: now}, false, "resolved"},
		{"a withdrawn approval is never withdrawn again", superseded, ApprovalRecord{Status: "withdrawn"}, true, "resolved"},
		{"an expired approval is resolved", fresh, ApprovalRecord{Status: "expired", ExpiresAt: now}, true, "resolved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ApprovalDisposition(test.row, test.approval, ApprovalResolution{Approved: test.approved, Now: now}); got != test.want {
				t.Fatalf("ApprovalDisposition = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompensationFailureNamesAMissingOrForeignTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		commandTenant string
		found         bool
		want          string
	}{
		{"the command is missing", "tenant", false, "compensation_target_missing"},
		{"the command belongs to another tenant", "other", true, "compensation_tenant_mismatch"},
		{"the command belongs to the tenant", "tenant", true, ""},
	}
	for _, test := range tests {
		if got := CompensationFailure("tenant", test.commandTenant, test.found); got != test.want {
			t.Errorf("%s: CompensationFailure = %q, want %q", test.name, got, test.want)
		}
	}
}
