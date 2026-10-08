package domain

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

const testIdempotency = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func mustDigest(t *testing.T, digest string) []byte {
	t.Helper()
	raw, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func marshal(t *testing.T, document map[string]any) []byte {
	t.Helper()
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// commandRow builds a schema-valid, digest-bound command ledger row.
func commandRow(t *testing.T) CommandRow {
	t.Helper()
	document := map[string]any{
		"command_id": "cmd-1", "intent_id": "int-1", "tenant_id": "tenant", "effector_route": "maintenance.ticket",
		"normalized_target": "motor/1", "idempotency_key": testIdempotency, "status": "prepared",
		"payload": map[string]any{"reason": "test"}, "created_at": testNow.Format(time.RFC3339Nano),
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, document)
	if err != nil {
		t.Fatal(err)
	}
	return CommandRow{ID: "cmd-1", TenantID: "tenant", IntentID: "int-1", Route: "maintenance.ticket", Target: "motor/1",
		JSON: marshal(t, document), SHA: mustDigest(t, digest), Idempotency: mustDigest(t, testIdempotency)}
}

func intentRow(t *testing.T, risk string) IntentRow {
	t.Helper()
	document := map[string]any{
		"intent_id": "int-1", "decision_id": "dec-1", "tenant_id": "tenant", "situation_id": "sit-1", "situation_version": 1,
		"type": "maintenance.ticket", "risk_class": risk, "parameters": map[string]any{"target": "motor/1"},
		"expires_at": testNow.Add(time.Hour).Format(time.RFC3339Nano),
	}
	digest, err := contractsv1.IntentDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	document["intent_digest"] = digest
	return IntentRow{ID: "int-1", TenantID: "tenant", DecisionID: "dec-1", SituationID: "sit-1", Version: 1, Type: "maintenance.ticket",
		Risk: risk, ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339Nano), PolicyStatus: "approved",
		JSON: marshal(t, document), SHA: mustDigest(t, digest)}
}

func decisionRow(t *testing.T, intent IntentRow) DecisionRow {
	t.Helper()
	var document map[string]any
	if err := jsonUnmarshal(intent.JSON, &document); err != nil {
		t.Fatal(err)
	}
	decision := map[string]any{
		"decision_id": "dec-1", "episode_id": "epi-1", "attempt_id": "att-1", "fence": 1,
		"snapshot_digest": "sha256:" + hex.EncodeToString(make([]byte, 32)), "situation_id": "sit-1", "situation_version": 1,
		"confidence": 0.9, "intents": []any{document},
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatal(err)
	}
	return DecisionRow{ValidationStatus: "accepted", SituationID: "sit-1", SituationVersion: 1, EpisodeID: "epi-1",
		JSON: marshal(t, decision), SHA: mustDigest(t, digest)}
}

// authorizationRecords builds a fully current, approved set of records.
func authorizationRecords(t *testing.T, risk string) AuthorizationRecords {
	t.Helper()
	intent := intentRow(t, risk)
	return AuthorizationRecords{
		Command: commandRow(t), Intent: intent, Decision: decisionRow(t, intent),
		Approval:  ApprovalRow{ID: "apr-1", ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339Nano), Present: true},
		Episode:   EpisodeRow{TenantID: "tenant", SituationID: "sit-1", SituationVersion: 1, Lifecycle: "concluded"},
		Situation: SituationRow{TenantID: "tenant", LastMaterialVersion: 1},
	}
}
