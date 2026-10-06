package domain

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
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
				want = "calibration"
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
	base := IntentRecord{EpisodeLifecycle: "concluded", CurrentSituation: 1, SituationVersion: 1, RiskClass: "R2", CurrentCompleteness: "on_time", ExpiresAt: FormatTime(now.Add(time.Hour))}
	for _, tc := range []struct {
		name           string
		change         func(*IntentRecord)
		status, reason string
	}{
		{"healthy", func(*IntentRecord) {}, "", ""},
		{"lifecycle first", func(r *IntentRecord) { r.EpisodeLifecycle = "running"; r.CurrentSituation = 2; r.ExpiresAt = "invalid" }, "denied", "episode_not_concluded"},
		{"stale before health", func(r *IntentRecord) { r.CurrentSituation = 2; r.CurrentCompleteness = "uncertain" }, "stale", "situation_version_stale"},
		{"health before expiry", func(r *IntentRecord) { r.CurrentCompleteness = "uncertain"; r.ExpiresAt = "invalid" }, "denied", "source_health_incomplete"},
		{"expiry inclusive", func(r *IntentRecord) { r.ExpiresAt = FormatTime(now) }, "expired", "intent_expired"},
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
	a := ApprovalRecord{Status: "pending", ExpiresAt: FormatTime(now)}
	r := ApprovalResolution{Approved: true, Now: now}
	base.CurrentSituation = 2
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
	a.ExpiresAt = FormatTime(now.Add(time.Hour))
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
