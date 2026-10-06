package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestTargetResolutionPreservesFallbackPrecedence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		parameters any
		want       string
	}{
		{"target", map[string]any{"target": " motor/1 ", "entity_id": "other"}, "motor/1"},
		{"entity", map[string]any{"entity_id": " motor/2 "}, "motor/2"},
		{"nonstring target", map[string]any{"target": 1, "entity_id": "other"}, "other"},
		{"invalid target wins", map[string]any{"target": " ", "entity_id": "other"}, "intent"},
		{"control", map[string]any{"target": "motor\n1"}, "intent"},
		{"length", map[string]any{"target": strings.Repeat("a", 257)}, "intent"},
		{"missing", nil, "intent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizedTarget("intent", testParameters(tc.parameters)); got != tc.want {
				t.Fatalf("target=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestDecisionValidationPrecedenceAndBinding(t *testing.T) {
	t.Parallel()
	good := decisionRecord(t)
	for _, tc := range []struct {
		name   string
		change func(*IntentRecord)
		want   string
	}{
		{"accepted", func(*IntentRecord) {}, ""},
		{"rejection before schema", func(r *IntentRecord) { r.ValidationStatus = "rejected"; r.DecisionJSON = nil }, "decision_not_accepted"},
		{"malformed", func(r *IntentRecord) { r.DecisionJSON = []byte("{") }, "schema_invalid"},
		{"schema", func(r *IntentRecord) { r.DecisionJSON = []byte("{}") }, "schema_invalid"},
		{"tenant before digest", func(r *IntentRecord) { r.EpisodeTenant = "other"; r.DecisionSHA = nil }, "identity_mismatch"},
		{"situation", func(r *IntentRecord) { r.EpisodeVersion = 2 }, "identity_mismatch"},
		{"digest", func(r *IntentRecord) { r.DecisionSHA = make([]byte, 32) }, "decision_digest_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := good
			tc.change(&r)
			if _, got := ParseDecision(r); got != tc.want {
				t.Fatalf("reason=%q want=%q", got, tc.want)
			}
		})
	}
}

func decisionRecord(t *testing.T) IntentRecord {
	t.Helper()
	document := map[string]any{"decision_id": "decision", "episode_id": "episode", "attempt_id": "attempt", "fence": 1, "snapshot_digest": "sha256:" + strings.Repeat("0", 64), "situation_id": "situation", "situation_version": 1, "confidence": 0.9, "intents": []any{map[string]any{}}}
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	return IntentRecord{ValidationStatus: "accepted", DecisionJSON: raw, DecisionSHA: sha, DecisionID: "decision", EpisodeID: "episode", SituationID: "situation", SituationVersion: 1, EpisodeTenant: "tenant", TenantID: "tenant", SituationTenant: "tenant", DecisionSituation: "situation", DecisionVersion: 1, EpisodeSituation: "situation", EpisodeVersion: 1}
}

func TestIntentDigestChecksOriginalDocument(t *testing.T) {
	t.Parallel()
	document := map[string]any{"intent_id": "intent", "decision_id": "decision", "tenant_id": "tenant", "situation_id": "situation", "situation_version": 1, "type": "ticket", "risk_class": "R1", "parameters": map[string]any{}, "expires_at": "2099-01-01T00:00:00Z"}
	digest, err := contractsv1.IntentDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	document["intent_digest"] = digest
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	if !documentMatchesBytes(raw, sha, canonicaljson.DomainIntent) {
		t.Fatal("valid intent digest rejected")
	}
	document["type"] = "changed"
	raw, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if documentMatchesBytes(raw, sha, canonicaljson.DomainIntent) {
		t.Fatal("mutated intent digest accepted")
	}
	if documentMatchesBytes([]byte("{"), sha, canonicaljson.DomainIntent) || documentMatchesBytes(raw, nil, canonicaljson.DomainIntent) {
		t.Fatal("invalid document accepted")
	}
	document["type"] = "ticket"
	var decoded map[string]any
	raw, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	row := IntentRecord{IntentID: "intent", DecisionID: "decision", TenantID: "tenant", SituationID: "situation", SituationVersion: 1, IntentType: "ticket", RiskClass: "R1"}
	if !MatchesIntentIdentity(row, ProjectIntent(decoded)) {
		t.Fatal("identity rejected")
	}
	row.RiskClass = "R2"
	if MatchesIntentIdentity(row, ProjectIntent(decoded)) {
		t.Fatal("risk mismatch accepted")
	}
}

func TestRuleBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, status := range []string{"concluded", "closed", "running"} {
		if EpisodeConcluded(IntentRecord{EpisodeLifecycle: status}) != (status != "running") {
			t.Fatalf("lifecycle=%s", status)
		}
	}
	for _, risk := range []string{"R0", "R1", "R2", "R3", "R4"} {
		for _, health := range []string{"on_time", "provisional", "uncertain"} {
			want := risk >= "R2" && health != "on_time"
			if SourceHealthIncomplete(IntentRecord{RiskClass: risk, CurrentCompleteness: health}) != want {
				t.Fatalf("health=%s risk=%s", health, risk)
			}
		}
	}
	if !ApprovalExpired("invalid", now) || !ApprovalExpired(FormatTime(now), now) || ApprovalExpired(FormatTime(now.Add(time.Nanosecond)), now) {
		t.Fatal("expiry boundary changed")
	}
	for _, approved := range []bool{true, false} {
		status, policy := ApprovalDecision(approved)
		if approved && (status != "approved" || policy != "pending") || !approved && (status != "denied" || policy != "denied") {
			t.Fatal(status, policy)
		}
	}
}

func TestDefinitionAndAssertionCanonicalBytes(t *testing.T) {
	t.Parallel()
	if _, err := DigestForVersion(""); err == nil {
		t.Fatal("missing version accepted")
	}
	digest, err := DigestForVersion("v1")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := canonicaljson.Digest(canonicaljson.DomainPolicy, CanonicalDocumentForVersion("v1"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := DigestForVersion("v2")
	if err != nil {
		t.Fatal(err)
	}
	if digest != expected || digest == other {
		t.Fatal("definition binding changed")
	}
	assertion := ApprovalAssertion{ApprovalID: "approval", IntentID: "intent", ApproverID: "human", RelayID: "relay"}
	signed, err := ApprovalAssertionSigningBytes(assertion)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := canonicalApprovalAssertion(assertion)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(signed, append([]byte(canonicaljson.DomainApproval), raw...)) {
		t.Fatal("assertion domain changed")
	}
	assertion.Approved = !assertion.Approved
	opposite, err := ApprovalAssertionSigningBytes(assertion)
	if err != nil || bytes.Equal(signed, opposite) {
		t.Fatal("approval decision not signed", err)
	}
	assertion.Approved = !assertion.Approved
	assertion.RelayID = "other"
	changed, err := ApprovalAssertionSigningBytes(assertion)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(signed, changed) {
		t.Fatal("relay not bound")
	}
}

func documentMatchesBytes(raw, digest []byte, domain canonicaljson.Domain) bool {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil {
		return false
	}
	return DocumentDigestMatches(document, digest, domain)
}

func testParameters(value any) map[string]any { v, _ := value.(map[string]any); return v }
