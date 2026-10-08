package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

type admittedSituation struct {
	db              *storage.DB
	compiled        *spec.CompiledSpec
	asm             *app.Assembler
	version         situations.Version
	base            time.Time
	schedulerItemID string
}

func triggeredSpec(executor spec.Executor, intents ...spec.Intent) *spec.CompiledSpec {
	return &spec.CompiledSpec{
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
			Executor: executor,
		},
		Actions: spec.Actions{Intents: intents},
	}
}

func admitTriggeredSituation(t *testing.T, ctx context.Context, compiled *spec.CompiledSpec, situationID string) *admittedSituation {
	t.Helper()
	db := openMigratedDB(t, ctx)
	if err := spec.SaveDeployment(ctx, db, "default", compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	base := time.Now().UTC()
	v := situations.Version{
		SituationID:  situationID,
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EntityType:   "thing",
		EntityID:     "ent-1",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
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
		"SELECT scheduler_item_id FROM scheduler_items WHERE situation_id = ?", situationID,
	).Scan(&schedulerItemID); err != nil {
		t.Fatalf("query scheduler item: %v", err)
	}
	return &admittedSituation{
		db: db, compiled: compiled, asm: app.NewAssembler(compiled, sources.Deterministic()),
		version: v, base: base, schedulerItemID: schedulerItemID,
	}
}

func openMigratedDB(t *testing.T, ctx context.Context) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return db
}

func (s *admittedSituation) assemble(t *testing.T, ctx context.Context) *app.Request {
	t.Helper()
	var req *app.Request
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		req, err = s.asm.Assemble(ctx, store.Join(tx), s.schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble tx: %v", err)
	}
	return req
}

func (s *admittedSituation) assembleAndPersist(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := s.asm.Assemble(ctx, store.Join(tx), s.schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		if err := s.asm.Persist(ctx, store.Join(tx), req, s.base); err != nil {
			return fmt.Errorf("persist: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}
}

func seedEpisode(t *testing.T, ctx context.Context, db *storage.DB, episodeID string) {
	t.Helper()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES (?, 'sch-test', 'tenant', 'sit-test', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'admitted', 0, ?)`,
		episodeID, digest, digest, "2026-08-12T10:00:00Z"); err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	// P8 (freshness): the dispatch-time situation-version recheck reads the
	// live situations registry — seed the row the episode is bound to.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-test', 'tenant', 'dep-test', 'test', 'thing', 'ent-1', 0, 'occ-test', 1, 0, 'candidate', 'open',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`); err != nil {
		t.Fatalf("seed situation registry: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
}
