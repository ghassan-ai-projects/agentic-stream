package store

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func openPolicyFixture(t *testing.T, risk string, currentVersion, intentVersion int, expiresAt time.Time) (*storage.DB, string) {
	t.Helper()
	ctx := context.Background()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	intentID := "int-policy"
	decisionID := "dec-policy"
	episodeID := "epi-policy"
	situationID := "sit-policy"
	snapshotDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, map[string]any{"phase": "watch"})
	if err != nil {
		t.Fatalf("snapshot digest: %v", err)
	}
	intent := map[string]any{
		"intent_id": intentID, "decision_id": decisionID, "tenant_id": "tenant",
		"situation_id": situationID, "situation_version": intentVersion,
		"type": "create_ticket", "risk_class": risk, "parameters": map[string]any{"target": "motor/1"},
		"expires_at": kernel.FormatTime(expiresAt.UTC()),
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatalf("intent digest: %v", err)
	}
	intent["intent_digest"] = intentDigest
	intentJSON, err := canonicaljson.Marshal(intent)
	if err != nil {
		t.Fatalf("intent json: %v", err)
	}
	intentSHA, _ := canonicaljson.DecodeDigest(intentDigest)
	decision := map[string]any{
		"decision_id": decisionID, "episode_id": episodeID, "attempt_id": "att-policy", "fence": 1,
		"snapshot_digest": snapshotDigest, "situation_id": situationID, "situation_version": intentVersion,
		"confidence": 0.9, "intents": []any{intent},
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatalf("decision json: %v", err)
	}
	decisionDigest, _ := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	decisionSHA, _ := canonicaljson.DecodeDigest(decisionDigest)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, 'tenant', 'dep', 'test', 'motor', 'motor-1', 0, 'occ', ?, 'watch', 'open', ?, ?, ?, ?)`,
		situationID, currentVersion, "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert situation: %v", err)
	}
	snapshotSHA, err := canonicaljson.DecodeDigest(snapshotDigest)
	if err != nil {
		t.Fatalf("decode snapshot digest: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at) VALUES ('lineage-policy', ?, 1, ?, '2026-08-12T00:00:00Z')`, snapshotSHA, []byte(`{}`)); err != nil {
		t.Fatalf("insert policy lineage: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, phase, severity, confidence, completeness,
			event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES ('sit-policy', ?, 'watch', 30, 0.8, 'on_time', ?, ?, ?, ?, 'lineage-policy', ?)`,
		intentVersion, "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", []byte(`{"phase":"watch"}`), snapshotSHA, "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert policy situation version: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO roles (role_id, role_name) VALUES ('role-approver', 'approver')`); err != nil {
		t.Fatalf("insert approval role: %v", err)
	}
	publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	if _, err := db.ExecContext(ctx, `INSERT INTO principals (principal_id, tenant_id, status, created_at, public_key) VALUES ('operator-1', 'tenant', 'active', '2026-08-12T00:00:00Z', ?)`, []byte(publicKey)); err != nil {
		t.Fatalf("insert approver principal: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO principals (principal_id, tenant_id, status, created_at) VALUES ('relay-1', 'tenant', 'active', '2026-08-12T00:00:00Z')`); err != nil {
		t.Fatalf("insert relay principal: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO principal_roles (principal_id, role_id) VALUES ('operator-1', 'role-approver')`); err != nil {
		t.Fatalf("bind approval role: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approval_authorities (tenant_id, entity_id, risk_class, role_id) VALUES ('tenant', 'motor-1', 'R2', 'role-approver')`); err != nil {
		t.Fatalf("insert approval authority: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
			admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES (?, 'sch-policy', 'tenant', ?, ?, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
		episodeID, situationID, intentVersion, make([]byte, 32), make([]byte, 32), "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
			raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES (?, ?, 'att-policy', 1, 1, ?, ?, ?, ?, 'accepted', X'7B7D', ?)`,
		decisionID, episodeID, situationID, intentVersion, decisionJSON, decisionSHA, "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert decision: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type,
			risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
		) VALUES (?, ?, 'tenant', ?, ?, 'create_ticket', ?, ?, ?, ?, 'pending', ?, ?)`,
		intentID, decisionID, situationID, intentVersion, risk, intentJSON, intentSHA,
		kernel.FormatTime(expiresAt.UTC()), "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return db, intentID
}
