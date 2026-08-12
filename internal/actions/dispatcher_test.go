package actions_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type recordingEffector struct {
	calls   int
	unknown bool
}

type tripBeforeAcceptEffector struct {
	db    *storage.DB
	calls int
}

func (e *tripBeforeAcceptEffector) Dispatch(context.Context, actions.Command) (actions.Effect, error) {
	return actions.Effect{}, errors.New("unauthorized dispatch path")
}

func (e *tripBeforeAcceptEffector) DispatchAuthorized(ctx context.Context, command actions.Command, authorization actions.Authorization) (actions.Effect, error) {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		return interlock.Set(ctx, tx, "tripped", "race stop", 2, time.Now().UTC().Format(time.RFC3339Nano))
	}); err != nil {
		return actions.Effect{}, fmt.Errorf("trip interlock: %w", err)
	}
	if err := authorization.Check(ctx); err != nil {
		return actions.Effect{}, fmt.Errorf("check authorization: %w", err)
	}
	e.calls++
	return actions.Effect{ProviderResult: map[string]any{"accepted": true}}, nil
}

func (e *recordingEffector) Dispatch(_ context.Context, command actions.Command) (actions.Effect, error) {
	e.calls++
	if e.unknown {
		return actions.Effect{}, &actions.UnknownOutcomeError{Err: errors.New("provider timeout")}
	}
	return actions.Effect{ProviderResult: map[string]any{"accepted": true, "target": command.NormalizedTarget}}, nil
}

func TestDispatcherRecordsSuccessAndDoesNotRedispatchDeliveredOutbox(t *testing.T) {
	db, commandID := openActionFixture(t)
	defer func() { _ = db.Close() }()
	effector := &recordingEffector{}
	dispatcher := actions.NewDispatcher(db, effector, clock.Physical(), ids.Deterministic(), "test-dispatcher", time.Minute)

	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("first dispatch processed=%v err=%v", processed, err)
	}
	processed, err = dispatcher.DispatchOnce(context.Background())
	if err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	if processed || effector.calls != 1 {
		t.Fatalf("second dispatch processed=%v effector calls=%d", processed, effector.calls)
	}
	var commandStatus, outboxStatus, outcomeStatus, reconciliation string
	if err := db.QueryRowContext(context.Background(), `
		SELECT c.status, o.status, r.status, r.reconciliation_status
		FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id
		JOIN outcomes r ON r.command_id = c.command_id
		WHERE c.command_id = ?`, commandID).Scan(&commandStatus, &outboxStatus, &outcomeStatus, &reconciliation); err != nil {
		t.Fatalf("read success ledger: %v", err)
	}
	if commandStatus != "succeeded" || outboxStatus != "delivered" || outcomeStatus != "succeeded" || reconciliation != "observed" {
		t.Fatalf("success ledger command=%q outbox=%q outcome=%q reconciliation=%q", commandStatus, outboxStatus, outcomeStatus, reconciliation)
	}
}

func TestDispatcherDoesNotBlindlyRetryUnknownOutcome(t *testing.T) {
	db, commandID := openActionFixture(t)
	defer func() { _ = db.Close() }()
	effector := &recordingEffector{unknown: true}
	dispatcher := actions.NewDispatcher(db, effector, clock.Physical(), ids.Deterministic(), "test-dispatcher", time.Minute)

	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("unknown dispatch processed=%v err=%v", processed, err)
	}
	processed, err = dispatcher.DispatchOnce(context.Background())
	if err != nil {
		t.Fatalf("repeat unknown dispatch: %v", err)
	}
	if processed || effector.calls != 1 {
		t.Fatalf("unknown outcome was retried: processed=%v calls=%d", processed, effector.calls)
	}
	var commandStatus, outboxStatus, outcomeStatus, reconciliation string
	if err := db.QueryRowContext(context.Background(), `
		SELECT c.status, o.status, r.status, r.reconciliation_status
		FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id
		JOIN outcomes r ON r.command_id = c.command_id
		WHERE c.command_id = ?`, commandID).Scan(&commandStatus, &outboxStatus, &outcomeStatus, &reconciliation); err != nil {
		t.Fatalf("read unknown ledger: %v", err)
	}
	if commandStatus != "reconciling" || outboxStatus != "failed" || outcomeStatus != "unknown" || reconciliation != "required" {
		t.Fatalf("unknown ledger command=%q outbox=%q outcome=%q reconciliation=%q", commandStatus, outboxStatus, outcomeStatus, reconciliation)
	}
	if err := dispatcher.ReconcileUnknown(context.Background(), commandID, "succeeded", map[string]any{"provider_id": "p-1"}); err != nil {
		t.Fatalf("reconcile unknown outcome: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&commandStatus); err != nil {
		t.Fatalf("read reconciled command: %v", err)
	}
	var outcomeCount int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM outcomes WHERE command_id = ?", commandID).Scan(&outcomeCount); err != nil {
		t.Fatalf("count reconciled outcomes: %v", err)
	}
	if commandStatus != "succeeded" || outcomeCount != 2 {
		t.Fatalf("reconciled command=%q outcomes=%d", commandStatus, outcomeCount)
	}
}

func TestDispatcherRefusesCommandWhenInterlockTrips(t *testing.T) {
	db, commandID := openActionFixture(t)
	defer func() { _ = db.Close() }()
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		return interlock.Set(context.Background(), tx, "tripped", "maintenance stop", 2, time.Now().UTC().Format(time.RFC3339Nano))
	}); err != nil {
		t.Fatalf("trip interlock: %v", err)
	}
	effector := &recordingEffector{}
	dispatcher := actions.NewDispatcher(db, effector, clock.Physical(), ids.Deterministic(), "test-dispatcher", time.Minute).WithInterlock(interlock.DurableReader{})
	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("dispatch tripped command processed=%v err=%v", processed, err)
	}
	if effector.calls != 0 {
		t.Fatalf("effector was called despite tripped interlock: %d", effector.calls)
	}
	var status string
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil {
		t.Fatalf("read refused command: %v", err)
	}
	if status != "failed" {
		t.Fatalf("refused command status = %q, want failed", status)
	}
}

func TestDispatcherEffectorAcceptanceRechecksInterlock(t *testing.T) {
	db, commandID := openActionFixture(t)
	defer func() { _ = db.Close() }()
	effector := &tripBeforeAcceptEffector{db: db}
	dispatcher := actions.NewDispatcher(db, effector, clock.Physical(), ids.Deterministic(), "test-dispatcher", time.Minute).WithInterlock(interlock.DurableReader{})
	processed, err := dispatcher.DispatchOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("dispatch race command processed=%v err=%v", processed, err)
	}
	if effector.calls != 0 {
		t.Fatal("effector accepted command after interlock tripped")
	}
	var status string
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil {
		t.Fatalf("read race command: %v", err)
	}
	if status != "failed" {
		t.Fatalf("race command status = %q, want failed", status)
	}
}

func openActionFixture(t *testing.T) (*storage.DB, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "actions.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	now := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	commandID := "cmd-action"
	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	intent := map[string]any{
		"intent_id": "int-action", "decision_id": "dec-action", "tenant_id": "tenant",
		"situation_id": "sit-action", "situation_version": 1, "type": "maintenance.ticket",
		"risk_class": "R1", "parameters": map[string]any{"target": "motor/1"}, "expires_at": expiresAt,
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatalf("digest intent: %v", err)
	}
	intent["intent_digest"] = intentDigest
	intentJSON, err := canonicaljson.Marshal(intent)
	if err != nil {
		t.Fatalf("marshal intent: %v", err)
	}
	intentSHA := mustDecode(t, intentDigest)
	decision := map[string]any{
		"decision_id": "dec-action", "episode_id": "epi-action", "attempt_id": "att-action", "fence": 1,
		"snapshot_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"situation_id":    "sit-action", "situation_version": 1, "confidence": 0.9,
		"intents": []any{intent},
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatalf("digest decision: %v", err)
	}
	command := map[string]any{
		"command_id": commandID, "intent_id": "int-action", "tenant_id": "tenant",
		"effector_route": "maintenance.ticket", "normalized_target": "motor/1",
		"idempotency_key": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"status":          "prepared", "payload": map[string]any{"reason": "test"},
		"created_at": now,
	}
	commandJSON, err := canonicaljson.Marshal(command)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	commandDigest, err := canonicaljson.Digest(canonicaljson.DomainCommand, command)
	if err != nil {
		t.Fatalf("digest command: %v", err)
	}
	commandSHA, err := canonicaljson.DecodeDigest(commandDigest)
	if err != nil {
		t.Fatalf("decode command digest: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-action', 'tenant', 'dep', 'test', 'motor', 'motor-1', 0, 'occ-action', 1, 'watch', 'open', ?, ?, ?, ?)`,
		now, now, now, now); err != nil {
		t.Fatalf("insert situation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
			admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES ('epi-action', 'sch-action', 'tenant', 'sit-action', 1,
			'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
		make([]byte, 32), make([]byte, 32), now); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	decisionSHA := mustDecode(t, decisionDigest)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
			situation_version, raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES ('dec-action', 'epi-action', 'att-action', 1, 1, 'sit-action', 1, ?, ?, 'accepted', X'7B7D', ?)`,
		decisionJSON, decisionSHA, now); err != nil {
		t.Fatalf("insert decision: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES (?, 'int-action', 'tenant', 'maintenance.ticket', 'motor/1', ?, ?, ?, 'pending', ?, ?)`,
		commandID, mustDecode(t, command["idempotency_key"].(string)), commandJSON, commandSHA,
		now, now); err != nil {
		t.Fatalf("insert command: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO outbox (kind, aggregate_id, aggregate_version, payload_json, status, available_at, created_at)
		VALUES ('command', ?, 1, ?, 'pending', ?, ?)`, commandID, commandJSON,
		now, now); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version,
			intent_type, risk_class, intent_json, intent_sha256, expires_at,
			policy_status, created_at, updated_at
		) VALUES ('int-action', 'dec-action', 'tenant', 'sit-action', 1,
			'maintenance.ticket', 'R1', ?, ?, ?, 'approved', ?, ?)`,
		intentJSON, intentSHA, expiresAt, now, now); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return db, commandID
}

func mustDecode(t *testing.T, digest string) []byte {
	t.Helper()
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode digest: %v", err)
	}
	return decoded
}
