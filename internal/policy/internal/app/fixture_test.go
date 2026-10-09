package app_test

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	fixtureNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	farFuture  = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
)

func newTestService(t *testing.T, configure ...func(*policy.Config)) *policy.Service {
	t.Helper()
	cfg := policy.Config{PolicyVersion: "policy-v1", IDGenerator: sources.Deterministic(), RuntimeOwner: unownedCheck, DecisionEpoch: unownedCheck}
	for _, change := range configure {
		change(&cfg)
	}
	service, err := policy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func unownedCheck(context.Context, *sql.Tx, string) error { return nil }

func exec(t *testing.T, db *storage.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
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

func commandAndOutboxCounts(t *testing.T, db *storage.DB) (commands, outbox int) {
	t.Helper()
	return scalar[int](t, db, "SELECT COUNT(*) FROM commands"), scalar[int](t, db, "SELECT COUNT(*) FROM outbox")
}

func inTx(t *testing.T, db *storage.DB, run func(tx *sql.Tx) error) {
	t.Helper()
	if err := db.WithTx(t.Context(), run); err != nil {
		t.Fatal(err)
	}
}

func evaluateIntent(t *testing.T, db *storage.DB, service *policy.Service, intentID string, now time.Time) policy.Result {
	t.Helper()
	var result policy.Result
	inTx(t, db, func(tx *sql.Tx) error {
		var err error
		result, err = service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	})
	return result
}

// Changes to the world after an intent was proposed, as SQL a case applies
// before the intent is evaluated or its approval resolved.
const (
	tripInterlock           = "UPDATE runtime_interlock SET status = 'tripped', reason = 'operator stop' WHERE singleton_id = 1"
	newerMaterialVersion    = "UPDATE situations SET current_version = 2, last_material_version = 2 WHERE situation_id = 'sit-policy'"
	uncertainSourceHealth   = "UPDATE situation_versions SET completeness = 'uncertain'"
	abandonEpisode          = "UPDATE episodes SET lifecycle_status = 'abandoned'"
	requireCatalogApproval  = "UPDATE intents SET requires_approval = 1"
	revokeApprovalAuthority = "DELETE FROM approval_authorities"

	provisionalSourceHealth = `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at) VALUES ('lineage-health', zeroblob(32), 1, x'7b7d', '2026-08-12T00:00:00Z');
		UPDATE situation_versions SET completeness = 'provisional', snapshot_json = x'7b7d', snapshot_sha256 = zeroblob(32), lineage_id = 'lineage-health'`
)

// pendingApproval is an R2 intent on its current Situation version with its
// approval request open: operator-1 may approve it through relay-1.
type pendingApproval struct {
	db         *storage.DB
	service    *policy.Service
	intentID   string
	approvalID string
	now        time.Time
}

func openPendingApproval(t *testing.T) pendingApproval {
	t.Helper()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	return requestApproval(t, db, intentID)
}

func requestApproval(t *testing.T, db *storage.DB, intentID string) pendingApproval {
	t.Helper()
	service := newTestService(t)
	request := evaluateIntent(t, db, service, intentID, fixtureNow)
	if request.Result != "approval_required" || request.ApprovalID == "" {
		t.Fatalf("approval request = %+v", request)
	}
	return pendingApproval{db: db, service: service, intentID: intentID, approvalID: request.ApprovalID, now: fixtureNow}
}

func (p pendingApproval) status(t *testing.T) string {
	t.Helper()
	return scalar[string](t, p.db, "SELECT status FROM approvals WHERE approval_id = ?", p.approvalID)
}

func (p pendingApproval) lookup(approved bool) policy.ApprovalLookup {
	return policy.ApprovalLookup{ID: p.approvalID, TenantID: "tenant", Approver: "operator-1", Relay: "relay-1", Approved: approved}
}

func (p pendingApproval) presentation(t *testing.T, lookup policy.ApprovalLookup) (presentation policy.ApprovalPresentation, err error) {
	t.Helper()
	err = p.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		presentation, err = p.service.ApprovalForSigning(t.Context(), tx, lookup)
		return err
	})
	return presentation, err
}

func (p pendingApproval) signedResolution(t *testing.T, approved bool) policy.ApprovalResolution {
	t.Helper()
	presentation, err := p.presentation(t, p.lookup(approved))
	if err != nil {
		t.Fatalf("present approval: %v", err)
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), presentation.SigningBytes)
	return policy.ApprovalResolution{TenantID: "tenant", ID: p.approvalID, Approved: approved, Approver: "operator-1", Relay: "relay-1", Signature: signature, Reason: "operator reviewed immutable evidence", Now: p.now}
}

func (p pendingApproval) resolve(t *testing.T, resolution policy.ApprovalResolution) (policy.Result, error) {
	t.Helper()
	var result policy.Result
	err := p.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		result, err = p.service.ResolveApproval(t.Context(), tx, resolution)
		return err
	})
	return result, err
}

func (p pendingApproval) approve(t *testing.T) policy.Result {
	t.Helper()
	result, err := p.resolve(t, p.signedResolution(t, true))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return result
}

// openPolicyFixture seeds an accepted Decision with one pending intent of the
// given risk class on a Situation at currentVersion; the intent was proposed
// for intentVersion.
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
	inTx(t, db, func(tx *sql.Tx) error {
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
		return nil
	})
	provisionFixtureGovernance(t, db)
	exec(t, db, "PRAGMA foreign_keys = ON")
	return db, intentID
}

func setMaterialVersion(t *testing.T, db *storage.DB, version int) {
	t.Helper()
	exec(t, db, "UPDATE situations SET last_material_version = ? WHERE situation_id = 'sit-policy'", version)
}

// provisionFixtureGovernance registers relay-1, operator-1 with its Ed25519
// key, and operator-1's R2 authority on motor-1 through the provisioning use
// case operators run (`principals apply`).
func provisionFixtureGovernance(t *testing.T, db *storage.DB) {
	t.Helper()
	publicKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	document, err := policy.ParsePrincipals([]byte(`tenant: tenant
principals:
  - id: operator-1
    public_key: ` + base64.StdEncoding.EncodeToString(publicKey) + `
  - id: relay-1
roles:
  - id: role-approver
    name: approver
    members: [operator-1]
    authorities:
      - entity: motor-1
        risks: [R2]
`))
	if err != nil {
		t.Fatalf("parse fixture governance: %v", err)
	}
	fence := policy.Ownership{Check: unownedCheck, Epoch: "fixture"}
	inTx(t, db, func(tx *sql.Tx) error {
		_, err := policy.ApplyPrincipals(t.Context(), tx, fence, document, time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
		return err
	})
}

// resealIntent changes the proposed intent's document and re-seals it, and the
// Decision that carries it, so every digest still verifies.
func resealIntent(t *testing.T, db *storage.DB, intentID string, change func(intent map[string]any)) {
	t.Helper()
	var intent, decision map[string]any
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT intent_json FROM intents WHERE intent_id = ?", intentID), &intent); err != nil {
		t.Fatalf("decode intent: %v", err)
	}
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT raw_json FROM decisions WHERE decision_id = 'dec-policy'"), &decision); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	change(intent)
	delete(intent, "intent_digest")
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatalf("intent digest: %v", err)
	}
	intent["intent_digest"] = intentDigest
	decision["intents"] = []any{intent}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatalf("decision digest: %v", err)
	}
	intentRaw, intentSHA := sealed(t, intent, intentDigest)
	decisionRaw, decisionSHA := sealed(t, decision, decisionDigest)
	exec(t, db, "UPDATE intents SET intent_json = ?, intent_sha256 = ? WHERE intent_id = ?", intentRaw, intentSHA, intentID)
	exec(t, db, "UPDATE decisions SET raw_json = ?, decision_sha256 = ? WHERE decision_id = 'dec-policy'", decisionRaw, decisionSHA)
}

func sealed(t *testing.T, document map[string]any, digest string) (raw, sha []byte) {
	t.Helper()
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	sha, err = canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode digest: %v", err)
	}
	return raw, sha
}
