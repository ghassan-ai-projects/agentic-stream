package domain

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func TestRiskRulesRemainAuthoritative(t *testing.T) {
	t.Parallel()
	for _, risk := range []string{"R0", "R1", "R2", "R3", "R4", "unknown"} {
		for _, approval := range []int{0, 1} {
			route, reason := RiskRoute(IntentRecord{RiskClass: risk, RequiresApproval: approval})
			want := "denied"
			switch risk {
			case "R0", "R1":
				want = "automatic"
				if approval != 0 {
					want = "approval"
				}
			case "R2":
				want = "approval"
			}
			if route != want || want == "denied" && reason == "" || want != "denied" && reason != "" {
				t.Fatal(risk, approval, route, reason)
			}
		}
	}
}
func TestFreshnessAndApprovalPrecedence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	base := IntentRecord{EpisodeProducedDecision: true, CurrentSituation: 1, SituationVersion: 1, RiskClass: "R2", CurrentCompleteness: "on_time", ExpiresAt: now.Add(time.Hour)}
	for _, tc := range []struct {
		name           string
		change         func(*IntentRecord)
		status, reason string
	}{
		{"healthy", func(*IntentRecord) {}, "", ""},
		{"lifecycle first", func(r *IntentRecord) {
			r.EpisodeProducedDecision = false
			r.CurrentSituation, r.LastMaterialVersion = 2, 2
			r.ExpiresAt = time.Time{}
		}, "denied", "episode_not_concluded"},
		{"newer version that is not material stays fresh", func(r *IntentRecord) { r.CurrentSituation, r.LastMaterialVersion = 3, 1 }, "", ""},
		{"stale before health", func(r *IntentRecord) {
			r.CurrentSituation, r.LastMaterialVersion = 2, 2
			r.CurrentCompleteness = "uncertain"
		}, "stale", "situation_version_stale"},
		{"health before expiry", func(r *IntentRecord) { r.CurrentCompleteness = "uncertain"; r.ExpiresAt = time.Time{} }, "denied", "source_health_incomplete"},
		{"expiry inclusive", func(r *IntentRecord) { r.ExpiresAt = now }, "expired", "intent_expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.change(&r)
			status, reason, _ := FreshnessFailure(r, now)
			if status != tc.status || reason != tc.reason {
				t.Fatal(status, reason)
			}
		})
	}
	a := ApprovalRecord{Status: "pending", ExpiresAt: now}
	r := ApprovalResolution{Approved: true, Now: now}
	base.CurrentSituation = 2
	if ApprovalDisposition(base, a, r) != "expired" {
		t.Fatal("a newer version that is not material must not stale an approval")
	}
	base.LastMaterialVersion = 2
	if ApprovalDisposition(base, a, r) != "stale" {
		t.Fatal("stale precedence")
	}
	a.Status = "approved"
	if ApprovalDisposition(base, a, r) != "resolved" {
		t.Fatal("resolved precedence")
	}
	a.Status = "pending"
	r.Approved = false
	if ApprovalDisposition(base, a, r) != "expired" {
		t.Fatal("decline expiry")
	}
	a.ExpiresAt = now.Add(time.Hour)
	if ApprovalDisposition(base, a, r) != "authorize" {
		t.Fatal("fresh decline")
	}
	if CompensationFailure("tenant", "tenant", false) != "compensation_target_missing" || CompensationFailure("tenant", "other", true) != "compensation_tenant_mismatch" || CompensationFailure("tenant", "tenant", true) != "" {
		t.Fatal("compensation binding")
	}
}
func TestApprovalPrincipalAndSignatureRules(t *testing.T) {
	t.Parallel()
	for _, r := range []ApprovalResolution{{}, {Approver: "a", Relay: "a"}, {Approver: "a"}} {
		if DistinctPrincipals(r) == nil {
			t.Fatal("invalid principals accepted")
		}
	}
	if DistinctPrincipals(ApprovalResolution{Approver: "a", Relay: "b"}) != nil {
		t.Fatal("distinct principals rejected")
	}
	if ActiveRelay(1, false) != nil || ActiveRelay(0, false) == nil || ActiveRelay(1, true) == nil {
		t.Fatal("relay rule")
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	public := key.Public().(ed25519.PublicKey)
	if ValidApproverKey(public, false) != nil || ValidApproverKey(nil, false) == nil || ValidApproverKey(public, true) == nil {
		t.Fatal("key rule")
	}
	if AuthorizedApprover(1, false) != nil || AuthorizedApprover(0, false) == nil || AuthorizedApprover(1, true) == nil {
		t.Fatal("authority rule")
	}
	assertion := []byte("signed")
	digest, err := VerifyAssertion(public, assertion, ed25519.Sign(key, assertion))
	if err != nil || len(digest) != 32 {
		t.Fatal(digest, err)
	}
	if _, err := VerifyAssertion(public, []byte("changed"), ed25519.Sign(key, assertion)); err == nil {
		t.Fatal("signature replay accepted")
	}
	if _, err := VerifyAssertion(nil, assertion, nil); err == nil {
		t.Fatal("invalid key accepted")
	}
	if CompleteDigest(digest, "intent") != nil || CompleteDigest(nil, "intent") == nil {
		t.Fatal("digest completeness")
	}
	if ApprovalNonce("approval", "intent") != "f5c26a8b40b74dfb1f32a590b0d2a186a7ac779cd71e17e8702052d59d2dff70" || ApprovalNonce("other", "intent") == ApprovalNonce("approval", "intent") {
		t.Fatal("nonce binding")
	}
}
func TestTypedIntentRetainsDigestInput(t *testing.T) {
	t.Parallel()
	r := decisionRecord(t)
	intent := map[string]any{"intent_id": "intent", "decision_id": r.DecisionID, "tenant_id": r.TenantID, "situation_id": r.SituationID, "situation_version": r.SituationVersion, "type": "ticket", "risk_class": "R1", "parameters": map[string]any{"opaque": map[string]any{"value": true}}, "expires_at": "2099-01-01T00:00:00Z"}
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	intent["intent_digest"] = digest
	r.IntentJSON, err = canonicaljson.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	r.IntentSHA, err = canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	docs, reason := ParseGovernanceDocuments(r)
	if reason != "" || docs.Intent.ID != "intent" || docs.Decision.ID != r.DecisionID {
		t.Fatal(docs, reason)
	}
	sealed, err := canonicaljson.Marshal(docs.Intent.Document)
	if err != nil || !bytes.Equal(sealed, r.IntentJSON) {
		t.Fatal("digest input changed", err)
	}
	if docs.Intent.Parameters["opaque"].(map[string]any)["value"] != true {
		t.Fatal("opaque payload lost")
	}
	ambiguous := r
	ambiguous.IntentJSON = contractstest.AmbiguousKeyJSON(r.IntentJSON, "intent_id")
	if _, reason := ParseIntent(ambiguous); reason != "schema_invalid" {
		t.Fatal(reason)
	}
	r.IntentSHA = nil
	if _, reason := ParseIntent(r); reason != "intent_digest_mismatch" {
		t.Fatal(reason)
	}
	r.IntentJSON = []byte("{}")
	if _, reason := ParseIntent(r); reason != "schema_invalid" {
		t.Fatal(reason)
	}
	r.ValidationStatus = "rejected"
	if _, reason := ParseGovernanceDocuments(r); reason != "decision_not_accepted" {
		t.Fatal(reason)
	}
}

func TestPolicyDocumentAndRoutesAgreeWithTheRiskTable(t *testing.T) {
	t.Parallel()
	document := CanonicalDocumentForVersion("v1")
	riskPolicy := document["risk_policy"].(map[string]any)
	health := document["incomplete_source_health"].(map[string]any)
	for _, risk := range contractstest.RiskClasses() {
		for _, approval := range []int{0, 1} {
			route, _ := RiskRoute(IntentRecord{RiskClass: string(risk), RequiresApproval: approval})
			if want := contractsv1.RouteFor(contractsv1.RiskClass(risk), approval != 0); route != string(want) {
				t.Fatalf("%s %d: route %s, want %s", risk, approval, route, want)
			}
		}
		if got := riskPolicy[string(risk)]; got != string(contractsv1.RouteFor(contractsv1.RiskClass(risk), false)) {
			t.Fatalf("%s: document route %v", risk, got)
		}
		_, documented := health[string(risk)]
		incomplete := SourceHealthIncomplete(IntentRecord{RiskClass: string(risk), CurrentCompleteness: "uncertain"})
		if documented != incomplete {
			t.Fatalf("%s: document incomplete=%t, rule=%t", risk, documented, incomplete)
		}
	}
}

func TestPolicyDigestIsPinned(t *testing.T) {
	t.Parallel()
	digest, err := DigestForVersion("v1")
	if err != nil || digest != "sha256:473ca13620fa9323385b275037070a405ab5f1d9a3c911d1403d3475dc5fb75e" {
		t.Fatal(digest, err)
	}
}

func TestOnlyApprovableRisksMayBeGranted(t *testing.T) {
	t.Parallel()
	for _, name := range append(contractstest.RiskClasses(), "R5", "") {
		risk := contractsv1.RiskClass(name)
		authority := AuthorityEntry{Entity: "motor", Risks: []string{name}}
		if granted := authority.validate("operator") == nil; granted != risk.Approvable() {
			t.Fatalf("%q granted=%t approvable=%t", risk, granted, risk.Approvable())
		}
	}
}

func TestWithdrawnApprovalIsNeverWithdrawnAgainByThePolicyPath(t *testing.T) {
	t.Parallel()
	superseded := IntentRecord{SituationVersion: 1, LastMaterialVersion: 2}
	resolution := ApprovalResolution{Approved: true, Now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	if got := ApprovalDisposition(superseded, ApprovalRecord{Status: "withdrawn"}, resolution); got != "resolved" {
		t.Fatalf("disposition of a withdrawn approval = %q, want resolved", got)
	}
}
