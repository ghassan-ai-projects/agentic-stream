package episodes_test

import (
	"context"
	"database/sql"
	"encoding/json"
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

const testDigest = "0000000000000000000000000000000000000000000000000000000000000000"

func insertSituationVersion(ctx context.Context, tx *sql.Tx, v situations.Version, deploymentID, tenantID string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES ('lin_test', X'0000000000000000000000000000000000000000000000000000000000000000', 1, X'5B5D', datetime('now'))
		ON CONFLICT(lineage_id) DO NOTHING`); err != nil {
		return fmt.Errorf("insert lineage set: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version, last_reasoned_version,
			phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 'active', ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(situation_id) DO NOTHING`,
		v.SituationID, tenantID, deploymentID, "test", v.EntityType, v.EntityID,
		0, "occ-"+v.SituationID, v.Version, v.Phase,
		v.EventHorizon.Format(time.RFC3339Nano), v.EventHorizon.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("insert situation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase,
			severity, confidence, completeness, event_horizon, watermark,
			valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'lin_test', datetime('now'))`,
		v.SituationID, v.Version, v.Phase, v.PreviousPhase,
		v.Severity, v.Confidence, v.Completeness,
		v.EventHorizon.Format(time.RFC3339Nano), v.Watermark.Format(time.RFC3339Nano),
		v.EventHorizon.Format(time.RFC3339Nano), v.SnapshotJSON, make([]byte, 32),
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}

func TestAssemblerBuildsEpisodeRequest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
				Name:          "native",
				ModelPolicy:   "test-policy",
				PromptVersion: "prompt-v1",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "approval", RateLimitPerHour: 2},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1","phase":"candidate","severity":10,"facts":{"facts.level":15}}`),
		SnapshotSHA256: testDigest,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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
	var req *episodes.Request
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		req, err = asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble tx: %v", err)
	}

	if req.EpisodeID == "" {
		t.Fatal("expected episode id")
	}
	if req.SchedulerItemID != schedulerItemID {
		t.Fatalf("expected scheduler item id %s, got %s", schedulerItemID, req.SchedulerItemID)
	}
	if req.SituationID != v.SituationID {
		t.Fatalf("expected situation id %s, got %s", v.SituationID, req.SituationID)
	}
	if req.ExecutorName != "native" {
		t.Fatalf("expected executor native, got %s", req.ExecutorName)
	}
	if req.SnapshotSHA256 == "" {
		t.Fatal("expected snapshot sha256")
	}
	if len(req.AdmissionKey) != 32 {
		t.Fatalf("expected admission key length 32, got %d", len(req.AdmissionKey))
	}
	if len(req.RequestJSON) == 0 {
		t.Fatal("expected request json")
	}
}

func TestAssemblerPersistCreatesEpisode(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
				Name:          "native",
				ModelPolicy:   "test-policy",
				PromptVersion: "prompt-v1",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_ticket", Risk: "R1", Schema: "schemas/ticket.json"},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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
		if err := asm.Persist(ctx, tx, req, base); err != nil {
			return fmt.Errorf("persist: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}

	var episodeID, status string
	if err := db.QueryRowContext(ctx,
		"SELECT episode_id, status FROM episodes WHERE scheduler_item_id = ?", schedulerItemID,
	).Scan(&episodeID, &status); err != nil {
		t.Fatalf("query episode: %v", err)
	}
	if episodeID == "" {
		t.Fatal("expected episode id")
	}
	if status != "queued" {
		t.Fatalf("expected queued status, got %s", status)
	}

	var itemStatus string
	if err := db.QueryRowContext(ctx,
		"SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", schedulerItemID,
	).Scan(&itemStatus); err != nil {
		t.Fatalf("query scheduler item status: %v", err)
	}
	if itemStatus != "admitted" {
		t.Fatalf("expected admitted scheduler item, got %s", itemStatus)
	}
}

func TestAssemblerIsDeterministic(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
				Name:          "native",
				ModelPolicy:   "test-policy",
				PromptVersion: "prompt-v1",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_ticket", Risk: "R1", Schema: "schemas/ticket.json"},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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

	var req1, req2 *episodes.Request
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		req1, err = asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble 1: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble 1 tx: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		req2, err = asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble 2: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble 2 tx: %v", err)
	}

	if req1.SnapshotSHA256 != req2.SnapshotSHA256 {
		t.Fatalf("deterministic snapshot hash mismatch: %s vs %s", req1.SnapshotSHA256, req2.SnapshotSHA256)
	}
}

func TestAssemblerRequestContainsDelta(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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
	var req *episodes.Request
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		req, err = asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble tx: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatalf("unmarshal request json: %v", err)
	}
	if _, ok := payload["delta"]; !ok {
		t.Fatal("expected delta in request json")
	}
}

func TestAssemblerTenantMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1"}`),
		SnapshotSHA256: testDigest,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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
		_, err := asm.Assemble(ctx, tx, schedulerItemID, "other-tenant")
		if err == nil {
			return fmt.Errorf("expected tenant mismatch error")
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble tx: %v", err)
	}
}

func TestAssemblerPersistRejectsNonPending(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testDigest,
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1"}`),
		SnapshotSHA256: testDigest,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
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
		if err := asm.Persist(ctx, tx, req, base); err != nil {
			return fmt.Errorf("first persist: %w", err)
		}
		// Second persist should fail because scheduler item is no longer pending.
		if err := asm.Persist(ctx, tx, req, base); err == nil {
			return fmt.Errorf("expected error persisting non-pending item")
		}
		return nil
	}); err != nil {
		t.Fatalf("persist tx: %v", err)
	}
}
