package app_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var fixtureNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

var zeroDigest = "sha256:" + strings.Repeat("0", 64)

type fixtureSpec struct {
	now          time.Time
	policyDigest string
}

type fixtureDocuments struct {
	now, expiresAt                          string
	intentJSON, intentSHA                   []byte
	decisionJSON, decisionSHA               []byte
	commandJSON, commandSHA, idempotencyKey []byte
}

func openActionFixture(t *testing.T) (*storage.DB, string) {
	t.Helper()
	return openFixture(t, fixtureSpec{})
}

func openFixture(t *testing.T, spec fixtureSpec) (*storage.DB, string) {
	t.Helper()
	if spec.now.IsZero() {
		spec.now = fixtureNow
	}
	db := storagetest.OpenTempWithoutForeignKeys(t)
	documents := buildFixtureDocuments(t, spec)
	seedFixtureLedger(t, db, documents)
	execute(t, db, "PRAGMA foreign_keys = ON")
	return db, "cmd-action"
}

func buildFixtureDocuments(t *testing.T, spec fixtureSpec) fixtureDocuments {
	t.Helper()
	now := kernel.FormatTime(spec.now.Add(-time.Minute))
	expiresAt := kernel.FormatTime(spec.now.Add(time.Hour))
	intent := fixtureIntent(t, expiresAt)
	decision := map[string]any{
		"decision_id": "dec-action", "episode_id": "epi-action", "attempt_id": "att-action", "fence": 1,
		"snapshot_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"situation_id":    "sit-action", "situation_version": 1, "confidence": 0.9,
		"intents": []any{intent},
	}
	command := fixtureCommand(now, spec.policyDigest)
	return fixtureDocuments{
		now: now, expiresAt: expiresAt,
		intentJSON: marshal(t, intent), intentSHA: decodeDigest(t, intent["intent_digest"].(string)),
		decisionJSON: marshal(t, decision), decisionSHA: decodeDigest(t, digestOf(t, canonicaljson.DomainDecision, decision)),
		commandJSON: marshal(t, command), commandSHA: decodeDigest(t, digestOf(t, canonicaljson.DomainCommand, command)),
		idempotencyKey: decodeDigest(t, command["idempotency_key"].(string)),
	}
}

func fixtureIntent(t *testing.T, expiresAt string) map[string]any {
	t.Helper()
	intent := map[string]any{
		"intent_id": "int-action", "decision_id": "dec-action", "tenant_id": "tenant",
		"situation_id": "sit-action", "situation_version": 1, "type": "maintenance.ticket",
		"risk_class": "R1", "parameters": map[string]any{"target": "motor/1"}, "expires_at": expiresAt,
	}
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatalf("digest intent: %v", err)
	}
	intent["intent_digest"] = digest
	return intent
}

func fixtureCommand(createdAt, policyDigest string) map[string]any {
	command := map[string]any{
		"command_id": "cmd-action", "intent_id": "int-action", "tenant_id": "tenant",
		"effector_route": "maintenance.ticket", "normalized_target": "motor/1",
		"idempotency_key": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"status":          "prepared", "payload": map[string]any{"reason": "test"},
		"created_at": createdAt,
	}
	if policyDigest != "" {
		command["policy_digest"] = policyDigest
	}
	return command
}

func seedFixtureLedger(t *testing.T, db *storage.DB, d fixtureDocuments) {
	t.Helper()
	execute(t, db, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-action', 'tenant', 'dep', 'test', 'motor', 'motor-1', 0, 'occ-action', 1, 'watch', 'open', ?, ?, ?, ?)`,
		d.now, d.now, d.now, d.now)
	execute(t, db, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
			admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES ('epi-action', 'sch-action', 'tenant', 'sit-action', 1,
			'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
		make([]byte, 32), make([]byte, 32), d.now)
	execute(t, db, `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
			situation_version, raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES ('dec-action', 'epi-action', 'att-action', 1, 1, 'sit-action', 1, ?, ?, 'accepted', X'7B7D', ?)`,
		d.decisionJSON, d.decisionSHA, d.now)
	execute(t, db, `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES ('cmd-action', 'int-action', 'tenant', 'maintenance.ticket', 'motor/1', ?, ?, ?, 'pending', ?, ?)`,
		d.idempotencyKey, d.commandJSON, d.commandSHA, d.now, d.now)
	execute(t, db, `
		INSERT INTO outbox (kind, aggregate_id, aggregate_version, payload_json, status, available_at, created_at)
		VALUES ('command', 'cmd-action', 1, ?, 'pending', ?, ?)`, d.commandJSON, d.now, d.now)
	execute(t, db, `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version,
			intent_type, risk_class, intent_json, intent_sha256, expires_at,
			policy_status, created_at, updated_at
		) VALUES ('int-action', 'dec-action', 'tenant', 'sit-action', 1,
			'maintenance.ticket', 'R1', ?, ?, ?, 'approved', ?, ?)`,
		d.intentJSON, d.intentSHA, d.expiresAt, d.now, d.now)
}

func execute(t *testing.T, db *storage.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", strings.Fields(statement)[0], err)
	}
}

func marshal(t *testing.T, document map[string]any) []byte {
	t.Helper()
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	return encoded
}

func digestOf(t *testing.T, domain canonicaljson.Domain, document map[string]any) string {
	t.Helper()
	digest, err := canonicaljson.Digest(domain, document)
	if err != nil {
		t.Fatalf("digest document: %v", err)
	}
	return digest
}

func decodeDigest(t *testing.T, digest string) []byte {
	t.Helper()
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode digest: %v", err)
	}
	return decoded
}

type ledger struct {
	Command, Outbox, Outcome, Reconciliation, Verification string
	Outcomes                                               int
}

func readLedger(t *testing.T, db *storage.DB, commandID string) ledger {
	t.Helper()
	var got ledger
	var outcome, reconciliation, verification sql.NullString
	err := db.QueryRowContext(t.Context(), `
		SELECT c.status, o.status,
			(SELECT status FROM outcomes WHERE command_id = c.command_id ORDER BY ordinal DESC LIMIT 1),
			(SELECT reconciliation_status FROM outcomes WHERE command_id = c.command_id ORDER BY ordinal DESC LIMIT 1),
			(SELECT status FROM verifications WHERE command_id = c.command_id),
			(SELECT COUNT(*) FROM outcomes WHERE command_id = c.command_id)
		FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id
		WHERE c.command_id = ?`, commandID).Scan(&got.Command, &got.Outbox, &outcome, &reconciliation, &verification, &got.Outcomes)
	if err != nil {
		t.Fatalf("read ledger of %s: %v", commandID, err)
	}
	got.Outcome, got.Reconciliation, got.Verification = outcome.String, reconciliation.String, verification.String
	return got
}

func count(t *testing.T, db *storage.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func queryString(t *testing.T, db *storage.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}
