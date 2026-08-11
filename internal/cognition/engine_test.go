package cognition_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// testDigest is a valid 64-character hex SHA-256 digest for tests.
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
		v.EventHorizon.Format(time.RFC3339Nano), []byte("{}"), make([]byte, 32),
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}

func TestTriggerIgnoredWhenConditionFalse(t *testing.T) {
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
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	v := situations.Version{
		SituationID:   "sit-1",
		Version:       1,
		Phase:         "candidate",
		Severity:      10,
		Confidence:    1.0,
		Completeness:  "provisional",
		EventHorizon:  time.Now().UTC(),
		Watermark:     time.Now().UTC(),
		Facts:         map[string]any{"facts.level": 5.0},
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "ignored" {
		t.Fatalf("expected ignored, got %s", outcome)
	}
}

func TestTriggerAdmittedWhenConditionTrue(t *testing.T) {
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
					Name:        "high",
					When:        "features.level > 10",
					Score:       "situation.severity",
					Threshold:   5,
					Lane:        "fast",
					MaterialDelta: "delta.phase_changed",
				},
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
		SituationID:   "sit-1",
		Version:       1,
		Phase:         "candidate",
		Severity:      10,
		Confidence:    1.0,
		Completeness:  "provisional",
		EventHorizon:  base,
		Watermark:     base,
		Facts:         map[string]any{"facts.level": 15.0},
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var outcome, lane string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome, lane FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&outcome, &lane); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "admitted" {
		t.Fatalf("expected admitted, got %s", outcome)
	}
	if lane != "fast" {
		t.Fatalf("expected fast lane, got %s", lane)
	}
}

func TestDebounceSetsNotBefore(t *testing.T) {
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
					Debounce:  "5m",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewVirtual(base)
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clk)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	v := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var notBefore string
	if err := db.QueryRowContext(ctx,
		"SELECT not_before FROM scheduler_items WHERE situation_id = ?", v.SituationID,
	).Scan(&notBefore); err != nil {
		t.Fatalf("query not_before: %v", err)
	}
	want := base.Add(5 * time.Minute).Format(time.RFC3339Nano)
	if notBefore != want {
		t.Fatalf("expected not_before %s, got %s", want, notBefore)
	}
}

func TestCooldownDelaysNotBefore(t *testing.T) {
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
					Cooldown:  "10m",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewVirtual(base)
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clk)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	v1 := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}

	clk.Advance(2 * time.Minute)
	v2 := situations.Version{
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base.Add(2 * time.Minute),
		Watermark:    base.Add(2 * time.Minute),
		Facts:        map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v2)
	}); err != nil {
		t.Fatalf("process v2: %v", err)
	}

	var notBefore string
	if err := db.QueryRowContext(ctx,
		"SELECT not_before FROM scheduler_items WHERE situation_id = ? AND situation_version = ?",
		v2.SituationID, v2.Version,
	).Scan(&notBefore); err != nil {
		t.Fatalf("query not_before: %v", err)
	}
	want := base.Add(10 * time.Minute).Format(time.RFC3339Nano)
	if notBefore != want {
		t.Fatalf("expected not_before %s, got %s", want, notBefore)
	}
}

func TestCoalescingPendingItem(t *testing.T) {
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
	v1 := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}

	v2 := situations.Version{
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base.Add(time.Minute),
		Watermark:    base.Add(time.Minute),
		Facts:        map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v2)
	}); err != nil {
		t.Fatalf("process v2: %v", err)
	}

	var pending, coalesced int
	if err := db.QueryRowContext(ctx, `
		SELECT
			COUNT(CASE WHEN status = 'pending' THEN 1 END),
			COUNT(CASE WHEN status = 'coalesced' THEN 1 END)
		FROM scheduler_items WHERE situation_id = ?`, v1.SituationID,
	).Scan(&pending, &coalesced); err != nil {
		t.Fatalf("query items: %v", err)
	}
	if pending != 1 {
		t.Fatalf("expected 1 pending item, got %d", pending)
	}
	if coalesced != 1 {
		t.Fatalf("expected 1 coalesced item, got %d", coalesced)
	}
}

func TestCapacityExhaustionDefers(t *testing.T) {
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
	if err := fillPendingSchedulerItems(ctx, db, testDigest, "default", 100, base); err != nil {
		t.Fatalf("fill pending: %v", err)
	}

	v := situations.Version{
		SituationID:  "sit-cap",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ?", v.SituationID,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "deferred" {
		t.Fatalf("expected deferred, got %s", outcome)
	}

	var itemCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE situation_id = ?", v.SituationID,
	).Scan(&itemCount); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != 0 {
		t.Fatalf("expected no scheduler item for deferred trigger, got %d", itemCount)
	}
}

func TestMaterialDeltaFalseIgnores(t *testing.T) {
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
					Name:          "high",
					When:          "features.level > 10",
					Score:         "situation.severity",
					Threshold:     5,
					Lane:          "fast",
					MaterialDelta: "delta.phase_changed",
				},
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
	v1 := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}

	v2 := situations.Version{
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base.Add(time.Minute),
		Watermark:    base.Add(time.Minute),
		Facts:        map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v2)
	}); err != nil {
		t.Fatalf("process v2: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v2.SituationID, v2.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "ignored" {
		t.Fatalf("expected ignored for material delta false, got %s", outcome)
	}
}

func TestDeltaUsesPreviousVersion(t *testing.T) {
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
					Name:          "high",
					When:          "features.level > 10",
					Score:         "situation.severity",
					Threshold:     5,
					Lane:          "fast",
					MaterialDelta: "delta.severity_change == 10",
				},
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
	v1 := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}

	v2 := situations.Version{
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     20,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base.Add(time.Minute),
		Watermark:    base.Add(time.Minute),
		Facts:        map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v2)
	}); err != nil {
		t.Fatalf("process v2: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v2.SituationID, v2.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "admitted" {
		t.Fatalf("expected admitted when severity_change == 10, got %s", outcome)
	}
}

func fillPendingSchedulerItems(ctx context.Context, db *storage.DB, deploymentID, tenantID string, n int, base time.Time) error {
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
			VALUES ('lin_test', X'0000000000000000000000000000000000000000000000000000000000000000', 1, X'5B5D', datetime('now'))
			ON CONFLICT(lineage_id) DO NOTHING`); err != nil {
			return fmt.Errorf("insert lineage set: %w", err)
		}
		for i := 0; i < n; i++ {
			sitID := fmt.Sprintf("sit-fill-%d", i)
			trgID := fmt.Sprintf("trg_fill_%d", i)
			itemID := fmt.Sprintf("sch_fill_%d", i)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO situations (
					situation_id, tenant_id, deployment_id, situation_type, entity_type,
					entity_id, partition_id, occurrence_id, current_version, last_reasoned_version,
					phase, status, first_event_time, latest_event_time, updated_at, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 'active', ?, ?, datetime('now'), datetime('now'))`,
				sitID, tenantID, deploymentID, "test", "thing", fmt.Sprintf("ent-%d", i),
				0, "occ-"+sitID, 1, "candidate",
				base.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano),
			); err != nil {
				return fmt.Errorf("insert situation %d: %w", i, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO situation_versions (
					situation_id, version, previous_version, phase, previous_phase,
					severity, confidence, completeness, event_horizon, watermark,
					valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
				) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'lin_test', datetime('now'))`,
				sitID, 1, "candidate", "",
				10, 1.0, "provisional",
				base.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano),
				base.Format(time.RFC3339Nano), []byte("{}"), make([]byte, 32),
			); err != nil {
				return fmt.Errorf("insert version %d: %w", i, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO trigger_evaluations (
					trigger_id, tenant_id, deployment_id, trigger_name,
					situation_id, situation_version, score, threshold, lane,
					outcome, reasons_json, policy_sha256, evaluated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?, ?, ?)`,
				trgID, tenantID, deploymentID, "fill",
				sitID, 1, 10.0, 5.0, "fast",
				[]byte("[]"), make([]byte, 32),
				base.Format(time.RFC3339Nano),
			); err != nil {
				return fmt.Errorf("insert evaluation %d: %w", i, err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO scheduler_items (
					scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
					lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, datetime('now'), datetime('now'))`,
				itemID, trgID, tenantID, sitID, 1,
				"fast", 10.0, func(i int) []byte { h := sha256.Sum256([]byte(fmt.Sprintf("dedupe-%d", i))); return h[:] }(i),
				base.Format(time.RFC3339Nano), base.Add(15*time.Minute).Format(time.RFC3339Nano),
			); err != nil {
				return fmt.Errorf("insert item %d: %w", i, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("fill pending scheduler items: %w", err)
	}
	return nil
}

func TestSameVersionReevaluationUpserts(t *testing.T) {
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
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process v1 again: %v", err)
	}

	var itemCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&itemCount); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != 1 {
		t.Fatalf("expected one scheduler item after re-evaluation, got %d", itemCount)
	}
}

func TestScoreBelowThresholdIgnores(t *testing.T) {
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
					Threshold: 100,
					Lane:      "fast",
				},
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
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "ignored" {
		t.Fatalf("expected ignored below threshold, got %s", outcome)
	}
}

func TestPolicySHA256Stored(t *testing.T) {
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
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var stored []byte
	if err := db.QueryRowContext(ctx,
		"SELECT policy_sha256 FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&stored); err != nil {
		t.Fatalf("query policy sha256: %v", err)
	}
	want, err := hex.DecodeString(testDigest)
	if err != nil {
		t.Fatalf("decode test digest: %v", err)
	}
	if string(stored) != string(want) {
		t.Fatalf("expected policy_sha256 %x, got %x", want, stored)
	}
}

func TestEmptyMaterialDeltaDefaultsToMaterial(t *testing.T) {
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
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "admitted" {
		t.Fatalf("expected admitted with empty materialDelta, got %s", outcome)
	}
}

func TestDebounceAndCooldownTogether(t *testing.T) {
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
					Debounce:  "3m",
					Cooldown:  "10m",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewVirtual(base)
	eng, err := cognition.NewEngine(db, testDigest, "default", &compiled, ids.Deterministic(), clk)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	v1 := situations.Version{
		SituationID:  "sit-1",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.level": 15.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}

	clk.Advance(2 * time.Minute)
	v2 := situations.Version{
		SituationID:  "sit-1",
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base.Add(2 * time.Minute),
		Watermark:    base.Add(2 * time.Minute),
		Facts:        map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v2)
	}); err != nil {
		t.Fatalf("process v2: %v", err)
	}

	var notBefore string
	if err := db.QueryRowContext(ctx,
		"SELECT not_before FROM scheduler_items WHERE situation_id = ? AND situation_version = ?",
		v2.SituationID, v2.Version,
	).Scan(&notBefore); err != nil {
		t.Fatalf("query not_before: %v", err)
	}
	// Cooldown from v1 (base) dominates debounce from v2 (base+2m+3m = base+5m).
	want := base.Add(10 * time.Minute).Format(time.RFC3339Nano)
	if notBefore != want {
		t.Fatalf("expected not_before %s, got %s", want, notBefore)
	}
}
