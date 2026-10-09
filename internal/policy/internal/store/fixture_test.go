package store

import (
	"crypto/ed25519"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

// openPolicyFixture seeds an accepted Decision with one pending intent of the
// given risk class on a Situation at currentVersion; the intent was proposed
// for intentVersion. operator-1 holds R2 authority on motor-1 through relay-1.
func openPolicyFixture(t *testing.T, risk string, currentVersion, intentVersion int, expiresAt time.Time) (*storage.DB, string) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	intentID, decisionID, episodeID, situationID := "int-policy", "dec-policy", "epi-policy", "sit-policy"
	snapshotDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, map[string]any{"phase": "watch"})
	if err != nil {
		t.Fatalf("snapshot digest: %v", err)
	}
	snapshotSHA, err := canonicaljson.DecodeDigest(snapshotDigest)
	if err != nil {
		t.Fatalf("decode snapshot digest: %v", err)
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
	intentSHA, err := canonicaljson.DecodeDigest(intentDigest)
	if err != nil {
		t.Fatalf("decode intent digest: %v", err)
	}
	decision := map[string]any{
		"decision_id": decisionID, "episode_id": episodeID, "attempt_id": "att-policy", "fence": 1,
		"snapshot_digest": snapshotDigest, "situation_id": situationID, "situation_version": intentVersion,
		"confidence": 0.9, "intents": []any{intent},
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatalf("decision json: %v", err)
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatalf("decision digest: %v", err)
	}
	decisionSHA, err := canonicaljson.DecodeDigest(decisionDigest)
	if err != nil {
		t.Fatalf("decode decision digest: %v", err)
	}
	const created = "2026-08-12T00:00:00Z"
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		execIn(t, tx, `
			INSERT INTO situations (
				situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
				partition_id, occurrence_id, current_version, phase, status,
				first_event_time, latest_event_time, updated_at, created_at
			) VALUES (?, 'tenant', 'dep', 'test', 'motor', 'motor-1', 0, 'occ', ?, 'watch', 'open', ?, ?, ?, ?)`,
			situationID, currentVersion, created, created, created, created)
		execIn(t, tx, `INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at) VALUES ('lineage-policy', ?, 1, ?, ?)`, snapshotSHA, []byte(`{}`), created)
		execIn(t, tx, `
			INSERT INTO situation_versions (
				situation_id, version, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
			) VALUES (?, ?, 'watch', 30, 0.8, 'on_time', ?, ?, ?, ?, 'lineage-policy', ?)`,
			situationID, intentVersion, created, created, []byte(`{"phase":"watch"}`), snapshotSHA, created)
		execIn(t, tx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
				admission_key, request_json, lifecycle_status, current_fence, accepted_at
			) VALUES (?, 'sch-policy', 'tenant', ?, ?, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
			episodeID, situationID, intentVersion, make([]byte, 32), make([]byte, 32), created)
		execIn(t, tx, `
			INSERT INTO decisions (
				decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
				raw_json, decision_sha256, validation_status, validation_json, created_at
			) VALUES (?, ?, 'att-policy', 1, 1, ?, ?, ?, ?, 'accepted', X'7B7D', ?)`,
			decisionID, episodeID, situationID, intentVersion, decisionJSON, decisionSHA, created)
		execIn(t, tx, `
			INSERT INTO intents (
				intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type,
				risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
			) VALUES (?, ?, 'tenant', ?, ?, 'create_ticket', ?, ?, ?, ?, 'pending', ?, ?)`,
			intentID, decisionID, situationID, intentVersion, risk, intentJSON, intentSHA,
			kernel.FormatTime(expiresAt.UTC()), created, created)
		execIn(t, tx, `INSERT INTO roles (role_id, role_name) VALUES ('role-approver', 'approver')`)
		publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
		execIn(t, tx, `INSERT INTO principals (principal_id, tenant_id, status, created_at, public_key) VALUES ('operator-1', 'tenant', 'active', ?, ?)`, created, []byte(publicKey))
		execIn(t, tx, `INSERT INTO principals (principal_id, tenant_id, status, created_at) VALUES ('relay-1', 'tenant', 'active', ?)`, created)
		execIn(t, tx, `INSERT INTO principal_roles (principal_id, role_id) VALUES ('operator-1', 'role-approver')`)
		execIn(t, tx, `INSERT INTO approval_authorities (tenant_id, entity_id, risk_class, role_id) VALUES ('tenant', 'motor-1', 'R2', 'role-approver')`)
		return nil
	}); err != nil {
		t.Fatalf("seed policy fixture: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return db, intentID
}

var fixtureNow = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

// inJoined runs run on a Tx joined to a transaction the test commits.
func inJoined(t *testing.T, db *storage.DB, run func(tx *Tx) error) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(original *sql.Tx) error { return run(Join(original)) }); err != nil {
		t.Fatal(err)
	}
}

func execIn(t *testing.T, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func scalar[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

// copyIntent adds another intent row like intentID, with the given assignments
// (for example "tenant_id = 'other'") applied to the copy.
func copyIntent(t *testing.T, db *storage.DB, intentID, newID, assignments string) {
	t.Helper()
	statements := []string{
		"CREATE TEMP TABLE intent_copy AS SELECT * FROM intents WHERE intent_id = '" + intentID + "'",
		"UPDATE intent_copy SET intent_id = '" + newID + "', " + assignments,
		"INSERT INTO intents SELECT * FROM intent_copy",
		"DROP TABLE intent_copy",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("copy intent: %s: %v", statement, err)
		}
	}
}
