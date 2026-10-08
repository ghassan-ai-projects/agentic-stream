package store_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
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

type triggerFixture struct {
	t   *testing.T
	ctx context.Context
	db  *storage.DB
	eng *cognition.Service
}

func newTriggerFixture(t *testing.T, compiled spec.CompiledSpec, clock sources.Clock) *triggerFixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: &compiled, IDGen: sources.Deterministic(), Clock: clock})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return &triggerFixture{t: t, ctx: ctx, db: db, eng: eng}
}

func (f *triggerFixture) process(v situations.Version) {
	f.t.Helper()
	if err := f.db.WithTx(f.ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(f.ctx, tx, v, testSpecDigest, "default"); err != nil {
			return err
		}
		return f.eng.Process(f.ctx, tx, v)
	}); err != nil {
		f.t.Fatalf("process %s v%d: %v", v.SituationID, v.Version, err)
	}
}

func (f *triggerFixture) reprocess(v situations.Version) {
	f.t.Helper()
	if err := f.db.WithTx(f.ctx, func(tx *sql.Tx) error {
		return f.eng.Process(f.ctx, tx, v)
	}); err != nil {
		f.t.Fatalf("reprocess %s v%d: %v", v.SituationID, v.Version, err)
	}
}

func (f *triggerFixture) read(dest any, column, table string, v situations.Version) {
	f.t.Helper()
	query := "SELECT " + column + " FROM " + table + " WHERE situation_id = ? AND situation_version = ?"
	if err := f.db.QueryRowContext(f.ctx, query, v.SituationID, v.Version).Scan(dest); err != nil {
		f.t.Fatalf("query %s.%s: %v", table, column, err)
	}
}

func (f *triggerFixture) outcomeOf(v situations.Version) string {
	f.t.Helper()
	var outcome string
	f.read(&outcome, "outcome", "trigger_evaluations", v)
	return outcome
}

func (f *triggerFixture) notBeforeOf(v situations.Version) string {
	f.t.Helper()
	var notBefore string
	f.read(&notBefore, "not_before", "scheduler_items", v)
	return notBefore
}

func (f *triggerFixture) schedulerItemCount(v situations.Version) int {
	f.t.Helper()
	var count int
	f.read(&count, "COUNT(*)", "scheduler_items", v)
	return count
}

func (f *triggerFixture) requireOutcome(v situations.Version, want, reason string) {
	f.t.Helper()
	if got := f.outcomeOf(v); got != want {
		f.t.Fatalf("expected %s%s, got %s", want, reason, got)
	}
}

func highLevelTrigger() spec.Trigger {
	return spec.Trigger{
		Name:      "high",
		When:      "features.level > 10",
		Score:     "situation.severity",
		Threshold: 5,
		Lane:      "fast",
	}
}

func levelSpec(trigger spec.Trigger) spec.CompiledSpec {
	return spec.CompiledSpec{
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
		Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}},
	}
}

func heartbeatSpec() spec.CompiledSpec {
	return spec.CompiledSpec{
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
}

func levelVersion(version int, at time.Time, level float64) situations.Version {
	return situations.Version{
		SituationID:  "sit-1",
		Version:      version,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1.0,
		Completeness: "provisional",
		EventHorizon: at,
		Watermark:    at,
		Facts:        map[string]any{"facts.level": level},
	}
}

func TestTriggerIgnoredWhenConditionFalse(t *testing.T) {
	f := newTriggerFixture(t, levelSpec(highLevelTrigger()), sources.Physical())
	v := levelVersion(1, time.Now().UTC(), 5.0)

	f.process(v)

	f.requireOutcome(v, "ignored", "")
}

func TestTriggerHandlesNilFeatureValue(t *testing.T) {
	f := newTriggerFixture(t, heartbeatSpec(), sources.Physical())
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

	f.process(v)

	f.requireOutcome(v, "admitted", "")
}

func TestDebounceSetsNotBefore(t *testing.T) {
	trigger := highLevelTrigger()
	trigger.Debounce = "5m"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := newTriggerFixture(t, levelSpec(trigger), sources.NewVirtual(base))
	v := levelVersion(1, base, 15.0)

	f.process(v)

	want := base.Add(5 * time.Minute).Format(time.RFC3339Nano)
	if notBefore := f.notBeforeOf(v); notBefore != want {
		t.Fatalf("expected not_before %s, got %s", want, notBefore)
	}
}

func TestCooldownDelaysNotBefore(t *testing.T) {
	trigger := highLevelTrigger()
	trigger.Cooldown = "10m"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := sources.NewVirtual(base)
	f := newTriggerFixture(t, levelSpec(trigger), clk)
	f.process(levelVersion(1, base, 15.0))

	clk.Advance(2 * time.Minute)
	v2 := levelVersion(2, base.Add(2*time.Minute), 20.0)
	f.process(v2)

	want := base.Add(10 * time.Minute).Format(time.RFC3339Nano)
	if notBefore := f.notBeforeOf(v2); notBefore != want {
		t.Fatalf("expected not_before %s, got %s", want, notBefore)
	}
}

func TestMaterialDeltaFalseIgnores(t *testing.T) {
	trigger := highLevelTrigger()
	trigger.MaterialDelta = "delta.phase_changed"
	f := newTriggerFixture(t, levelSpec(trigger), sources.Physical())
	base := time.Now().UTC()
	f.process(levelVersion(1, base, 15.0))
	v2 := levelVersion(2, base.Add(time.Minute), 20.0)

	f.process(v2)

	f.requireOutcome(v2, "ignored", " for material delta false")
}

func TestDeltaUsesPreviousVersion(t *testing.T) {
	trigger := highLevelTrigger()
	trigger.MaterialDelta = "delta.severity_change == 10"
	f := newTriggerFixture(t, levelSpec(trigger), sources.Physical())
	base := time.Now().UTC()
	f.process(levelVersion(1, base, 15.0))
	v2 := levelVersion(2, base.Add(time.Minute), 20.0)
	v2.Severity = 20

	f.process(v2)

	f.requireOutcome(v2, "admitted", " when severity_change == 10")
}

func TestSameVersionReevaluationUpserts(t *testing.T) {
	f := newTriggerFixture(t, levelSpec(highLevelTrigger()), sources.Physical())
	v := levelVersion(1, time.Now().UTC(), 15.0)
	f.process(v)

	f.reprocess(v)

	if itemCount := f.schedulerItemCount(v); itemCount != 1 {
		t.Fatalf("expected one scheduler item after re-evaluation, got %d", itemCount)
	}
}

func TestScoreBelowThresholdIgnores(t *testing.T) {
	trigger := highLevelTrigger()
	trigger.Threshold = 100
	f := newTriggerFixture(t, levelSpec(trigger), sources.Physical())
	v := levelVersion(1, time.Now().UTC(), 15.0)

	f.process(v)

	f.requireOutcome(v, "ignored", " below threshold")
}

func TestPolicySHA256Stored(t *testing.T) {
	f := newTriggerFixture(t, levelSpec(highLevelTrigger()), sources.Physical())
	v := levelVersion(1, time.Now().UTC(), 15.0)
	f.process(v)

	var stored []byte
	f.read(&stored, "policy_sha256", "trigger_evaluations", v)

	want, err := hex.DecodeString(testDigest)
	if err != nil {
		t.Fatalf("decode test digest: %v", err)
	}
	if string(stored) != string(want) {
		t.Fatalf("expected policy_sha256 %x, got %x", want, stored)
	}
}

func TestEmptyMaterialDeltaDefaultsToMaterial(t *testing.T) {
	f := newTriggerFixture(t, levelSpec(highLevelTrigger()), sources.Physical())
	v := levelVersion(2, time.Now().UTC(), 15.0)

	f.process(v)

	f.requireOutcome(v, "admitted", " with empty materialDelta")
}
