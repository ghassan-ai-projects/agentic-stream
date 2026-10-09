package domain

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

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

// intentRecord is a decisionRecord that also carries a sealed R1 intent.
func intentRecord(t *testing.T, change func(intent map[string]any)) (IntentRecord, map[string]any) {
	t.Helper()
	row := decisionRecord(t)
	intent := map[string]any{"intent_id": "intent", "decision_id": row.DecisionID, "tenant_id": row.TenantID, "situation_id": row.SituationID, "situation_version": row.SituationVersion, "type": "ticket", "risk_class": "R1", "parameters": map[string]any{"opaque": map[string]any{"value": true}}, "expires_at": "2099-01-01T00:00:00Z"}
	if change != nil {
		change(intent)
	}
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	intent["intent_digest"] = digest
	if row.IntentJSON, err = canonicaljson.Marshal(intent); err != nil {
		t.Fatal(err)
	}
	if row.IntentSHA, err = canonicaljson.DecodeDigest(digest); err != nil {
		t.Fatal(err)
	}
	return row, intent
}

func TestDecisionValidationPrecedenceAndBinding(t *testing.T) {
	t.Parallel()
	good := decisionRecord(t)
	tests := []struct {
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
		{"ambiguous bytes", func(r *IntentRecord) { r.DecisionJSON = contractstest.AmbiguousKeyJSON(r.DecisionJSON, "decision_id") }, "schema_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			row := good
			test.change(&row)
			if _, got := ParseDecision(row); got != test.want {
				t.Fatalf("reason = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTheTypedIntentRetainsTheDigestInput(t *testing.T) {
	t.Parallel()
	row, _ := intentRecord(t, nil)

	docs, reason := ParseGovernanceDocuments(row)
	if reason != "" || docs.Intent.ID != "intent" || docs.Decision.ID != row.DecisionID {
		t.Fatalf("documents = %+v, reason %q", docs, reason)
	}
	sealedAgain, err := canonicaljson.Marshal(docs.Intent.Document)
	if err != nil || !bytes.Equal(sealedAgain, row.IntentJSON) {
		t.Fatalf("the retained document no longer matches the sealed bytes: %v", err)
	}
	if docs.Intent.Parameters["opaque"].(map[string]any)["value"] != true {
		t.Fatal("effector parameters are not kept opaque")
	}
}

func TestIntentValidationPrecedenceAndBinding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*IntentRecord)
		want   string
	}{
		{"ambiguous bytes", func(r *IntentRecord) { r.IntentJSON = contractstest.AmbiguousKeyJSON(r.IntentJSON, "intent_id") }, "schema_invalid"},
		{"missing digest", func(r *IntentRecord) { r.IntentSHA = nil }, "intent_digest_mismatch"},
		{"another digest", func(r *IntentRecord) { r.IntentSHA = make([]byte, 32) }, "intent_digest_mismatch"},
		{"not an intent", func(r *IntentRecord) { r.IntentJSON = []byte("{}") }, "schema_invalid"},
		{"not JSON", func(r *IntentRecord) { r.IntentJSON = []byte("{") }, "schema_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			row, _ := intentRecord(t, nil)
			test.change(&row)
			if _, got := ParseIntent(row); got != test.want {
				t.Fatalf("reason = %q, want %q", got, test.want)
			}
		})
	}
	t.Run("a rejected decision fails the pair before the intent is read", func(t *testing.T) {
		t.Parallel()
		row, _ := intentRecord(t, nil)
		row.ValidationStatus = "rejected"
		if _, got := ParseGovernanceDocuments(row); got != "decision_not_accepted" {
			t.Fatalf("reason = %q, want decision_not_accepted", got)
		}
	})
}

func TestAnIntentRowMustMatchItsSignedDocument(t *testing.T) {
	t.Parallel()
	row, _ := intentRecord(t, nil)
	row.IntentID, row.IntentType, row.RiskClass = "intent", "ticket", "R1"
	docs, reason := ParseGovernanceDocuments(row)
	if reason != "" {
		t.Fatal(reason)
	}
	tests := []struct {
		name   string
		change func(*IntentRecord)
		want   bool
	}{
		{"identical", func(*IntentRecord) {}, true},
		{"another risk class", func(r *IntentRecord) { r.RiskClass = "R2" }, false},
		{"another intent type", func(r *IntentRecord) { r.IntentType = "page" }, false},
		{"another intent id", func(r *IntentRecord) { r.IntentID = "other" }, false},
		{"another tenant", func(r *IntentRecord) { r.TenantID = "other" }, false},
		{"another Situation version", func(r *IntentRecord) { r.SituationVersion = 2 }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			changed := row
			test.change(&changed)
			if got := MatchesIntentIdentity(changed, docs.Intent); got != test.want {
				t.Fatalf("MatchesIntentIdentity = %t, want %t", got, test.want)
			}
		})
	}
}
