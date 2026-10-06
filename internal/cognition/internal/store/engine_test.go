package store_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// testDigest is the internal snapshot digest used by test rows.
const testDigest = "0000000000000000000000000000000000000000000000000000000000000000"

const testSpecDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

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
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		EventHorizon: time.Now().UTC(),
		Watermark:    time.Now().UTC(),
		Facts:        map[string]any{"facts.level": 5.0},
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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

func TestTriggerHandlesNilFeatureValue(t *testing.T) {
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
		Operators: []spec.Operator{
			{Name: "heartbeat_missing", Kind: "missing_heartbeat", Output: "heartbeat_missing_5m"},
		},
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "warning",
			Phases:       []spec.Phase{{Name: "warning", Severity: 60}},
			Reducers: []spec.Reducer{
				{Field: "facts.heartbeat_missing_5m", Strategy: "latest_event_time", Input: "heartbeat_missing_5m"},
			},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{
				{
					Name:      "warning_needs_diagnosis",
					When:      `situation.phase == "warning" && !features.heartbeat_missing_5m`,
					Score:     "double(situation.severity)",
					Threshold: 45,
					Lane:      "deep",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	base := time.Now().UTC()
	v := situations.Version{
		SituationID:  "sit-nil-feature",
		Version:      1,
		Phase:        "warning",
		Severity:     60,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: base,
		Watermark:    base,
		Facts:        map[string]any{"facts.heartbeat_missing_5m": nil},
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v)
	}); err != nil {
		t.Fatalf("process nil feature: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE situation_id = ? AND situation_version = ?",
		v.SituationID, v.Version,
	).Scan(&outcome); err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "admitted" {
		t.Fatalf("expected admitted, got %s", outcome)
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
					Debounce:  "5m",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := sources.NewVirtual(base)
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: clk})
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
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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
					Cooldown:  "10m",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := sources.NewVirtual(base)
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: clk})
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
		if err := insertSituationVersion(ctx, tx, v1, testSpecDigest, "default"); err != nil {
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
		if err := insertSituationVersion(ctx, tx, v2, testSpecDigest, "default"); err != nil {
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
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v1, testSpecDigest, "default"); err != nil {
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
		if err := insertSituationVersion(ctx, tx, v2, testSpecDigest, "default"); err != nil {
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
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v1, testSpecDigest, "default"); err != nil {
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
		if err := insertSituationVersion(ctx, tx, v2, testSpecDigest, "default"); err != nil {
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
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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
					Threshold: 100,
					Lane:      "fast",
				},
			},
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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
		},
	}

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: sources.Physical()})
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
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
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
