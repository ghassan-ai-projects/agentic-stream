package episodes_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRunnerExecutesAdmittedEpisode(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testSpecDigest,
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
			Reducers: []spec.Reducer{
				{Field: "facts.level", Strategy: "latest_event_time", Input: "level"},
			},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{
				{
					Name:      "high",
					When:      "features.level > 10",
					Score:     "situation.severity",
					Threshold: 5,
					Lane:      "fast",
				},
			},
			Executor: spec.Executor{
				Name:          "fake",
				ModelPolicy:   "test-policy",
				PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision.",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_maintenance_ticket", Risk: "R1", Schema: "schemas/ticket.json"},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testSpecDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	base := time.Now().UTC()
	v := situations.Version{
		SituationID:    "sit-1",
		Version:        1,
		Phase:          "candidate",
		Severity:       10,
		Confidence:     1.0,
		Completeness:   "provisional",
		EntityType:     "thing",
		EntityID:       "ent-1",
		EventHorizon:   base,
		Watermark:      base,
		Facts:          map[string]any{"facts.level": 15.0},
		SnapshotJSON:   []byte(`{"situation_id":"sit-1","phase":"candidate"}`),
		SnapshotSHA256: testDigest,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var schedulerItemID string
	if err := db.QueryRowContext(ctx,
		"SELECT scheduler_item_id FROM scheduler_items WHERE situation_id = ?", v.SituationID,
	).Scan(&schedulerItemID); err != nil {
		t.Fatalf("query scheduler item: %v", err)
	}

	asm := episodes.NewAssembler(&compiled, ids.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return asm.Persist(ctx, tx, req, base)
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}

	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	ran, err := runner.RunOnce(ctx, "default")
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !ran {
		t.Fatal("expected runner to process an episode")
	}

	var status string
	if err := db.QueryRowContext(ctx,
		"SELECT lifecycle_status FROM episodes WHERE scheduler_item_id = ?", schedulerItemID,
	).Scan(&status); err != nil {
		t.Fatalf("query episode status: %v", err)
	}
	if status != "concluded" {
		t.Fatalf("expected concluded, got %s", status)
	}

	var decisionCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM decisions WHERE situation_id = ?", v.SituationID,
	).Scan(&decisionCount); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if decisionCount != 1 {
		t.Fatalf("expected one decision, got %d", decisionCount)
	}
	var validationStatus, attemptID string
	var fence int64
	if err := db.QueryRowContext(ctx, `
		SELECT validation_status, attempt_id, fence
		FROM decisions WHERE situation_id = ?`, v.SituationID).Scan(&validationStatus, &attemptID, &fence); err != nil {
		t.Fatalf("query decision provenance: %v", err)
	}
	if validationStatus != "accepted" || attemptID == "" || fence != 1 {
		t.Fatalf("decision provenance = status %q attempt %q fence %d", validationStatus, attemptID, fence)
	}
	var attemptStatus string
	if err := db.QueryRowContext(ctx,
		"SELECT status FROM episode_attempts WHERE attempt_id = ?", attemptID).Scan(&attemptStatus); err != nil {
		t.Fatalf("query attempt status: %v", err)
	}
	if attemptStatus != "produced" {
		t.Fatalf("expected produced attempt, got %s", attemptStatus)
	}
	var intentType, policyStatus string
	if err := db.QueryRowContext(ctx, `
		SELECT intent_type, policy_status FROM intents
		WHERE decision_id = (SELECT decision_id FROM decisions WHERE situation_id = ?)`, v.SituationID).
		Scan(&intentType, &policyStatus); err != nil {
		t.Fatalf("query validated intent: %v", err)
	}
	if intentType != "create_maintenance_ticket" || policyStatus != "pending" {
		t.Fatalf("validated intent = type %q policy %q", intentType, policyStatus)
	}
}

func TestRunnerNoWorkWhenEmpty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	ran, err := runner.RunOnce(ctx, "default")
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if ran {
		t.Fatal("expected no work")
	}
}

type failOnceExecutor struct {
	calls    int
	delegate *episodes.FakeExecutor
}

func (e *failOnceExecutor) Name() string { return "fail-once" }

func (e *failOnceExecutor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	e.calls++
	if e.calls == 1 {
		return nil, fmt.Errorf("transient worker failure")
	}
	outcome, err := e.delegate.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("fail-once delegate: %w", err)
	}
	return outcome, nil
}

func TestRunnerRetriesFailedAttemptWithNextFence(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "retry.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys for fixture: %v", err)
	}
	digest := make([]byte, 32)
	acceptedAt := "2026-08-12T12:00:00Z"
	requestJSON := []byte(`{"snapshot":{"phase":"candidate"},"trigger":{"trigger_name":"retry"},"allowed_intent_types":["create_maintenance_ticket"],"risk_ceiling":"R1"}`)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
			admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES ('epi-retry', 'sch-retry', 'tenant', 'sit-retry', 1,
			'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'admitted', 0, ?)`,
		digest, digest, requestJSON, acceptedAt); err != nil {
		t.Fatalf("insert episode fixture: %v", err)
	}

	executor := &failOnceExecutor{delegate: episodes.NewFakeExecutor()}
	runner := episodes.NewRunner(db, executor, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("first run processed=%v err=%v", processed, err)
	}
	var lifecycle string
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-retry'").Scan(&lifecycle); err != nil {
		t.Fatalf("read lifecycle after failure: %v", err)
	}
	if lifecycle != "running" {
		t.Fatalf("lifecycle after failed attempt = %q, want running", lifecycle)
	}
	var failedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episode_attempts WHERE episode_id = 'epi-retry' AND status = 'failed'").Scan(&failedCount); err != nil {
		t.Fatalf("count failed attempts: %v", err)
	}
	if failedCount != 1 {
		t.Fatalf("failed attempts = %d, want 1", failedCount)
	}

	processed, err = runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("retry run processed=%v err=%v", processed, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-retry'").Scan(&lifecycle); err != nil {
		t.Fatalf("read lifecycle after retry: %v", err)
	}
	if lifecycle != "concluded" {
		t.Fatalf("lifecycle after retry = %q, want concluded", lifecycle)
	}
	var attempts, decisions, currentFence, producedFence int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(current_fence) FROM episodes WHERE episode_id = 'epi-retry'").Scan(&attempts, &currentFence); err != nil {
		t.Fatalf("read retry fence: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE episode_id = 'epi-retry'").Scan(&decisions); err != nil {
		t.Fatalf("count decisions after retry: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT fence FROM episode_attempts WHERE episode_id = 'epi-retry' AND status = 'produced'").Scan(&producedFence); err != nil {
		t.Fatalf("read produced fence: %v", err)
	}
	if attempts != 1 || currentFence != 2 || producedFence != 2 || decisions != 1 {
		t.Fatalf("retry state attempts=%d current_fence=%d produced_fence=%d decisions=%d", attempts, currentFence, producedFence, decisions)
	}
}
