package policy

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestGatewayAutomaticCommandIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := NewGateway("policy-v1", ids.Deterministic())

	var first Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		first, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("evaluate intent: %v", err)
	}
	if first.Result != "approved" || first.CommandID == "" {
		t.Fatalf("first result = %+v", first)
	}

	var second Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		second, err = gateway.EvaluateIntent(ctx, tx, intentID, now.Add(time.Second))
		return err
	}); err != nil {
		t.Fatalf("repeat evaluation: %v", err)
	}
	var commandCount, outboxCount, auditCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commands WHERE intent_id = ?", intentID).Scan(&commandCount); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE kind = 'command' AND aggregate_id = ?", first.CommandID).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_evaluations WHERE intent_id = ?", intentID).Scan(&auditCount); err != nil {
		t.Fatalf("count policy audit: %v", err)
	}
	if commandCount != 1 || outboxCount != 1 || auditCount != 2 || second.Result != "approved" {
		t.Fatalf("idempotency counts command=%d outbox=%d audits=%d second=%+v", commandCount, outboxCount, auditCount, second)
	}
}

func TestGatewayRiskFreshnessAndExpiry(t *testing.T) {
	tests := []struct {
		name           string
		risk           string
		currentVersion int
		intentVersion  int
		expiresAt      time.Time
		wantResult     string
		wantReason     string
	}{
		{name: "approval", risk: "R2", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "approval_required", wantReason: "risk_requires_approval"},
		{name: "deny high risk", risk: "R3", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "denied", wantReason: "risk_policy_denied"},
		{name: "stale", risk: "R1", currentVersion: 2, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "stale", wantReason: "situation_version_stale"},
		{name: "expired", risk: "R1", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), wantResult: "expired", wantReason: "intent_expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			db, intentID := openPolicyFixture(t, test.risk, test.currentVersion, test.intentVersion, test.expiresAt)
			defer func() { _ = db.Close() }()
			var result Result
			now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				var err error
				result, err = NewGateway("policy-v1", ids.Deterministic()).EvaluateIntent(ctx, tx, intentID, now)
				return err
			}); err != nil {
				t.Fatalf("evaluate intent: %v", err)
			}
			if result.Result != test.wantResult || result.Reason != test.wantReason {
				t.Fatalf("result = %+v, want %s/%s", result, test.wantResult, test.wantReason)
			}
		})
	}
}

func TestGatewayResolvesApprovalBeforeCommanding(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := NewGateway("policy-v1", ids.Deterministic())
	var approval Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		approval, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if approval.Result != "approval_required" || approval.ApprovalID == "" {
		t.Fatalf("approval result = %+v", approval)
	}
	var resolved Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		var nonce string
		if err := tx.QueryRowContext(ctx, "SELECT nonce FROM approvals WHERE approval_id = ?", approval.ApprovalID).Scan(&nonce); err != nil {
			return fmt.Errorf("load approval nonce: %w", err)
		}
		var intentSHA, decisionSHA []byte
		if err := tx.QueryRowContext(ctx, "SELECT i.intent_sha256, d.decision_sha256 FROM intents i JOIN decisions d ON d.decision_id = i.decision_id WHERE i.intent_id = ?", intentID).Scan(&intentSHA, &decisionSHA); err != nil {
			return fmt.Errorf("load approval digests: %w", err)
		}
		assertion, err := ApprovalAssertionSigningBytes(ApprovalAssertion{
			ApprovalID: approval.ApprovalID, IntentID: intentID, DecisionID: "dec-policy", TenantID: "tenant",
			SituationID: "sit-policy", SituationVersion: 1, RiskClass: "R2",
			IntentDigest: "sha256:" + hex.EncodeToString(intentSHA), DecisionDigest: "sha256:" + hex.EncodeToString(decisionSHA),
			ExpiresAt: "2099-01-01T00:00:00Z", Nonce: nonce, ApproverID: "operator-1", RelayID: "relay-1",
		})
		if err != nil {
			return err
		}
		privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
		resolved, err = gateway.ResolveApproval(ctx, tx, approval.ApprovalID, true, "operator-1", "relay-1", ed25519.Sign(privateKey, assertion), "approved for maintenance", now)
		return err
	}); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
	if resolved.Result != "approved" || resolved.CommandID == "" {
		t.Fatalf("resolved result = %+v", resolved)
	}
}

func TestGatewayRejectsSamePrincipalRelay(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := NewGateway("policy-v1", ids.Deterministic())
	var approval Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		approval, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		resolved, err := gateway.ResolveApproval(ctx, tx, approval.ApprovalID, true, "operator-1", "operator-1", nil, "invalid", now)
		if err != nil {
			return err
		}
		if resolved.Result != "denied" || resolved.Reason != "approval_principal_not_authorized" {
			t.Fatalf("resolved approval = %+v", resolved)
		}
		return nil
	}); err != nil {
		t.Fatalf("resolve unauthorized approval: %v", err)
	}
}

func TestGatewayFailsClosedWhenInterlockTripped(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := NewGateway("policy-v1", ids.Deterministic()).WithInterlock(interlock.DurableReader{})
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := interlock.Set(ctx, tx, "tripped", "operator stop", 2, now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("trip interlock: %w", err)
		}
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("evaluate tripped intent: %v", err)
	}
	if result.Result != "denied" || result.Reason != "interlock_not_ready" {
		t.Fatalf("tripped interlock result = %+v", result)
	}
}

func TestGatewayDeniesConsequentialIntentWhenCompletenessIsProvisional(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, "INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at) VALUES ('lineage-health', ?, 1, ?, '2026-08-12T00:00:00Z')", digest, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, phase, severity, confidence, completeness,
			event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES ('sit-policy', 1, 'watch', 30, 0.8, 'provisional', ?, ?, ?, ?, 'lineage-health', ?)`,
		"2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", []byte("{}"), digest, "2026-08-12T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = NewGateway("policy-v1", ids.Deterministic()).EvaluateIntent(ctx, tx, intentID, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "denied" || result.Reason != "source_health_incomplete" {
		t.Fatalf("result = %+v, want source_health_incomplete denial", result)
	}
}

func openPolicyFixture(t *testing.T, risk string, currentVersion, intentVersion int, expiresAt time.Time) (*storage.DB, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "policy.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
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
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano),
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
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, 'tenant', 'dep', 'test', 'motor', 'motor-1', 0, 'occ', ?, 'watch', 'open', ?, ?, ?, ?)`,
		situationID, currentVersion, "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert situation: %v", err)
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
		episodeID, situationID, intentVersion, digest, digest, "2026-08-12T00:00:00Z"); err != nil {
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
		expiresAt.UTC().Format(time.RFC3339Nano), "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z"); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return db, intentID
}
