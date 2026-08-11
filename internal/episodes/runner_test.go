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
				PromptVersion: "prompt-v1",
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
