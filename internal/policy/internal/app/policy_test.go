package app_test

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestGatewayAutomaticCommandIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := newTestService(t)

	first := evaluateIntent(t, db, gateway, intentID, now)
	if first.Result != "approved" || first.CommandID == "" {
		t.Fatalf("first result = %+v", first)
	}
	var commandJSON []byte
	if err := db.QueryRowContext(ctx, "SELECT command_json FROM commands WHERE command_id = ?", first.CommandID).Scan(&commandJSON); err != nil {
		t.Fatalf("load generated command: %v", err)
	}
	var commandDocument map[string]any
	if err := json.Unmarshal(commandJSON, &commandDocument); err != nil {
		t.Fatalf("decode generated command: %v", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaCommand, commandDocument); err != nil {
		t.Fatalf("generated command must validate: %v", err)
	}
	if commandDocument["policy_digest"] == "" || commandDocument["not_before_mono_us"] != float64(0) {
		t.Fatalf("generated command missing device freshness/policy binding: %#v", commandDocument)
	}

	second := evaluateIntent(t, db, gateway, intentID, now.Add(time.Second))
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
		// materialVersion is the Situation's latest material version
		// (ADR-018); zero means the current version.
		materialVersion int
		expiresAt       time.Time
		wantResult      string
		wantReason      string
	}{
		{name: "approval", risk: "R2", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "approval_required", wantReason: "risk_requires_approval"},
		{name: "deny high risk", risk: "R3", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "denied", wantReason: "risk_policy_denied"},
		{name: "stale", risk: "R1", currentVersion: 2, intentVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "stale", wantReason: "situation_version_stale"},
		{name: "newer version that is not material", risk: "R1", currentVersion: 3, intentVersion: 1, materialVersion: 1, expiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), wantResult: "approved", wantReason: "automatic_r0_r1"},
		{name: "expired", risk: "R1", currentVersion: 1, intentVersion: 1, expiresAt: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), wantResult: "expired", wantReason: "intent_expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, intentID := openPolicyFixture(t, test.risk, test.currentVersion, test.intentVersion, test.expiresAt)
			defer func() { _ = db.Close() }()
			setMaterialVersion(t, db, cmp.Or(test.materialVersion, test.currentVersion))
			now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			result := evaluateIntent(t, db, newTestService(t), intentID, now)
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
	gateway := newTestService(t)
	approval := evaluateIntent(t, db, gateway, intentID, now)
	if approval.Result != "approval_required" || approval.ApprovalID == "" {
		t.Fatalf("approval result = %+v", approval)
	}
	var requestedType string
	if err := db.QueryRowContext(ctx, "SELECT event_type FROM notifications WHERE event_id = ?", "approval.requested:"+approval.ApprovalID).Scan(&requestedType); err != nil {
		t.Fatalf("read approval notification: %v", err)
	}
	if requestedType != (notify.ApprovalRequested{}).EventType() {
		t.Fatalf("approval notification type=%q", requestedType)
	}
	resolved := resolveApproval(t, db, gateway, approval.ApprovalID, "approved for maintenance", now)
	if resolved.Result != "approved" || resolved.CommandID == "" {
		t.Fatalf("resolved result = %+v", resolved)
	}
}

// TestEvaluateIntentDeniesHighRiskDespiteRequiresApproval guards A-041 F1: the
// R3/R4 denial in the risk-class policy document is authoritative and must be
// enforced before any requires_approval routing.
func TestEvaluateIntentDeniesHighRiskDespiteRequiresApproval(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	for _, risk := range []string{"R3", "R4"} {
		t.Run(risk, func(t *testing.T) {
			db, intentID := openPolicyFixture(t, risk, 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
			defer func() { _ = db.Close() }()
			if _, err := db.ExecContext(ctx, "UPDATE intents SET requires_approval = 1 WHERE intent_id = ?", intentID); err != nil {
				t.Fatalf("set requires_approval: %v", err)
			}
			result := evaluateIntent(t, db, newTestService(t), intentID, now)
			if result.Result != "denied" || result.Reason != "risk_policy_denied" {
				t.Fatalf("result = %+v, want denied/risk_policy_denied", result)
			}
		})
	}
}

// TestEvaluateIntentApprovedRequiresApprovalDispatches guards A-041 F1: an
// approved R0/R1 requires_approval intent must terminate in dispatch, not spawn
// another approval request. Previously ResolveApproval reset the intent to
// pending and re-ran EvaluateIntent, which created a fresh approval each cycle
// (an unbounded approve -> re-pending loop).
func TestEvaluateIntentApprovedRequiresApprovalDispatches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "UPDATE intents SET requires_approval = 1 WHERE intent_id = ?", intentID); err != nil {
		t.Fatalf("set requires_approval: %v", err)
	}
	// The fixture seeds an R2 approval authority; this intent is R1, so grant the
	// approver authority for R1 too.
	if _, err := db.ExecContext(ctx, `INSERT INTO approval_authorities (tenant_id, entity_id, risk_class, role_id) VALUES ('tenant', 'motor-1', 'R1', 'role-approver')`); err != nil {
		t.Fatalf("insert R1 approval authority: %v", err)
	}
	gateway := newTestService(t)

	approval := evaluateIntent(t, db, gateway, intentID, now)
	if approval.Result != "approval_required" || approval.ApprovalID == "" {
		t.Fatalf("approval result = %+v, want an approval request", approval)
	}

	resolved := resolveApproval(t, db, gateway, approval.ApprovalID, "approved", now)
	if resolved.Result != "approved" || resolved.CommandID == "" {
		t.Fatalf("resolved result = %+v, want approved with a command (no re-pending loop)", resolved)
	}

	// Exactly one approval must exist for the intent; the loop would have
	// appended more on each cycle.
	var approvalCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM approvals WHERE intent_id = ?", intentID).Scan(&approvalCount); err != nil {
		t.Fatalf("count approvals: %v", err)
	}
	if approvalCount != 1 {
		t.Fatalf("approval count = %d, want exactly 1", approvalCount)
	}
}

func TestGatewayRejectsSamePrincipalRelay(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := newTestService(t)
	approval := evaluateIntent(t, db, gateway, intentID, now)
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := gateway.ResolveApproval(ctx, tx, policy.ApprovalResolution{TenantID: "tenant", ID: approval.ApprovalID, Approved: true, Approver: "operator-1", Relay: "operator-1", Now: now})
		return err
	})
	if !errors.Is(err, policy.ErrApprovalUnauthorized) {
		t.Fatalf("unauthorized resolution: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id = ?", approval.ApprovalID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatal("unauthorized submission consumed request", status)
	}

}

func TestGatewayFailsClosedWhenInterlockTripped(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	gateway := newTestService(t)
	var result policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := interlock.TripIn(ctx, tx, "operator stop", now); err != nil {
			return fmt.Errorf("trip interlock: %w", err)
		}
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
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
	if _, err := db.ExecContext(ctx, `UPDATE situation_versions SET completeness = 'provisional', snapshot_json = ?, snapshot_sha256 = ?, lineage_id = 'lineage-health' WHERE situation_id = 'sit-policy' AND version = 1`, []byte("{}"), digest); err != nil {
		t.Fatal(err)
	}
	result := evaluateIntent(t, db, newTestService(t), intentID, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if result.Result != "denied" || result.Reason != "source_health_incomplete" {
		t.Fatalf("result = %+v, want source_health_incomplete denial", result)
	}
}

func evaluateIntent(t *testing.T, db *storage.DB, gateway *policy.Service, intentID string, now time.Time) policy.Result {
	t.Helper()
	var result policy.Result
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	}); err != nil {
		t.Fatalf("evaluate intent %s: %v", intentID, err)
	}
	return result
}

func resolveApproval(t *testing.T, db *storage.DB, gateway *policy.Service, approvalID, reason string, now time.Time) policy.Result {
	t.Helper()
	ctx := t.Context()
	var resolved policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		presentation, err := gateway.ApprovalForSigning(ctx, tx, policy.ApprovalLookup{ID: approvalID, TenantID: "tenant", Approver: "operator-1", Relay: "relay-1", Approved: true})
		if err != nil {
			return err
		}
		privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
		resolved, err = gateway.ResolveApproval(ctx, tx, policy.ApprovalResolution{TenantID: "tenant", ID: approvalID, Approved: true, Approver: "operator-1", Relay: "relay-1", Signature: ed25519.Sign(privateKey, presentation.SigningBytes), Reason: reason, Now: now})
		return err
	}); err != nil {
		t.Fatalf("resolve approval %s: %v", approvalID, err)
	}
	return resolved
}

func openPolicyFixture(t *testing.T, risk string, currentVersion, intentVersion int, expiresAt time.Time) (*storage.DB, string) {
	t.Helper()
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

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
	provisionFixtureGovernance(t, db)
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

// setMaterialVersion records the fixture Situation's latest material version,
// which cognition writes in production (ADR-018).
func setMaterialVersion(t *testing.T, db *storage.DB, version int) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "UPDATE situations SET last_material_version = ? WHERE situation_id = 'sit-policy'", version); err != nil {
		t.Fatalf("set material version: %v", err)
	}
}

// provisionFixtureGovernance registers the relay, the approver with its
// Ed25519 key and the approver's R2 authority on motor-1 through the
// provisioning use case operators run (`principals apply`).
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
	fence := policy.Ownership{Check: func(context.Context, *sql.Tx, string) error { return nil }, Epoch: "fixture"}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := policy.ApplyPrincipals(t.Context(), tx, fence, document, time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("provision fixture governance: %v", err)
	}
}
