package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	testSpecDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	testTenant     = "default"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type harness struct {
	t     *testing.T
	db    *storage.DB
	svc   *Service
	clock *sources.Virtual
}

func fastTrigger() spec.Trigger {
	return spec.Trigger{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}
}

func levelSpec(trigger spec.Trigger) *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testSpecDigest,
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
			Reducers:     []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
		},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}},
	}
}

func newHarness(t *testing.T, compiled *spec.CompiledSpec) *harness {
	t.Helper()
	db := storagetest.OpenTemp(t)
	if err := spec.SaveDeployment(t.Context(), db, testTenant, compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	clock := sources.NewVirtual(base)
	svc, err := New(Config{DeploymentID: compiled.Digest, TenantID: testTenant, Spec: compiled, IDGen: sources.Deterministic(), Clock: clock})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{t: t, db: db, svc: svc, clock: clock}
}

func newTriggerHarness(t *testing.T, trigger spec.Trigger) *harness {
	t.Helper()
	return newHarness(t, levelSpec(trigger))
}

func candidateVersion(situationID string, version int, level float64) situations.Version {
	at := base.Add(time.Duration(version) * time.Minute)
	return situations.Version{
		SituationID: situationID, Version: version, Phase: "candidate", Severity: 10, Confidence: 1.0,
		Completeness: "provisional", EventHorizon: at, Watermark: at, Facts: map[string]any{"facts.level": level},
	}
}

func (h *harness) process(v situations.Version) {
	h.t.Helper()
	if err := h.tryProcess(v); err != nil {
		h.t.Fatalf("process %s v%d: %v", v.SituationID, v.Version, err)
	}
}

func (h *harness) tryProcess(v situations.Version) error {
	h.t.Helper()
	return h.db.WithTx(h.t.Context(), func(tx *sql.Tx) error {
		if err := insertSituationVersion(h.t.Context(), tx, v); err != nil {
			return err
		}
		return h.svc.Process(h.t.Context(), store.Join(tx), v)
	})
}

func (h *harness) reprocess(v situations.Version) {
	h.t.Helper()
	if err := h.db.WithTx(h.t.Context(), func(tx *sql.Tx) error { return h.svc.Process(h.t.Context(), store.Join(tx), v) }); err != nil {
		h.t.Fatalf("reprocess %s v%d: %v", v.SituationID, v.Version, err)
	}
}

func (h *harness) exec(statement string, args ...any) {
	h.t.Helper()
	if _, err := h.db.ExecContext(h.t.Context(), statement, args...); err != nil {
		h.t.Fatalf("%s: %v", statement, err)
	}
}

func scalar[T any](h *harness, query string, args ...any) T {
	h.t.Helper()
	var value T
	if err := h.db.QueryRowContext(h.t.Context(), query, args...).Scan(&value); err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
	return value
}

func (h *harness) evaluation(v situations.Version) domain.TriggerEvaluationRecord {
	h.t.Helper()
	records, err := store.NewReader(h.db.DB).TriggerEvaluations(h.t.Context(), testTenant, v.SituationID, v.Version)
	if err != nil || len(records) != 1 {
		h.t.Fatalf("evaluations of %s v%d = %+v, %v; want exactly one", v.SituationID, v.Version, records, err)
	}
	return records[0]
}

func (h *harness) itemCount(v situations.Version) int {
	h.t.Helper()
	return scalar[int](h, "SELECT COUNT(*) FROM scheduler_items WHERE situation_id = ? AND situation_version = ?", v.SituationID, v.Version)
}

func (h *harness) itemStatuses(situationID string) map[int]string {
	h.t.Helper()
	rows, err := h.db.QueryContext(h.t.Context(), "SELECT situation_version, status FROM scheduler_items WHERE situation_id = ?", situationID)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	statuses := map[int]string{}
	for rows.Next() {
		var version int
		var status string
		if err := rows.Scan(&version, &status); err != nil {
			h.t.Fatal(err)
		}
		statuses[version] = status
	}
	if err := rows.Err(); err != nil {
		h.t.Fatal(err)
	}
	return statuses
}

func (h *harness) notBefore(v situations.Version) sql.NullString {
	h.t.Helper()
	return scalar[sql.NullString](h, "SELECT not_before FROM scheduler_items WHERE situation_id = ? AND situation_version = ?", v.SituationID, v.Version)
}

func insertSituationVersion(ctx context.Context, tx *sql.Tx, v situations.Version) error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
			VALUES ('lin_test', zeroblob(32), 1, X'5B5D', datetime('now')) ON CONFLICT(lineage_id) DO NOTHING`, nil},
		{`INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
			current_version, last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, ?, ?, 'test', ?, ?, 0, ?, ?, 0, ?, 'active', ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(situation_id) DO UPDATE SET current_version = excluded.current_version`, []any{v.SituationID, testTenant, testSpecDigest, v.EntityType, v.EntityID, "occ-" + v.SituationID, v.Version, v.Phase,
			kernel.FormatTime(v.EventHorizon), kernel.FormatTime(v.EventHorizon)}},
		{`INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase, severity, confidence, completeness,
			event_horizon, watermark, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, X'7B7D', zeroblob(32), 'lin_test', datetime('now'))`, []any{v.SituationID, v.Version, v.Phase, v.PreviousPhase,
			v.Severity, v.Confidence, v.Completeness, kernel.FormatTime(v.EventHorizon), kernel.FormatTime(v.Watermark), kernel.FormatTime(v.EventHorizon)}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("seed %q: %w", statement.sql[:30], err)
		}
	}
	return nil
}

func (h *harness) fillQueue(pending int) {
	h.t.Helper()
	err := h.db.WithTx(h.t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(h.t.Context(), `INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
			VALUES ('lin_test', zeroblob(32), 1, X'5B5D', datetime('now')) ON CONFLICT(lineage_id) DO NOTHING`); err != nil {
			return fmt.Errorf("seed lineage: %w", err)
		}
		for i := range pending {
			if err := insertPendingItem(h.t.Context(), tx, i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.t.Fatalf("fill queue: %v", err)
	}
}

func insertPendingItem(ctx context.Context, tx *sql.Tx, i int) error {
	situation, trigger, item := fmt.Sprintf("sit-fill-%d", i), fmt.Sprintf("trg_fill_%d", i), fmt.Sprintf("sch_fill_%d", i)
	dedupe := sha256.Sum256([]byte(item))
	at := kernel.FormatTime(base)
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
			current_version, last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, ?, ?, 'test', 'thing', ?, 0, ?, 1, 0, 'candidate', 'active', ?, ?, datetime('now'), datetime('now'))`,
			[]any{situation, testTenant, testSpecDigest, "ent-" + situation, "occ-" + situation, at, at}},
		{`INSERT INTO situation_versions (
			situation_id, version, phase, previous_phase, severity, confidence, completeness, event_horizon, watermark,
			valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES (?, 1, 'candidate', '', 10, 1.0, 'provisional', ?, ?, ?, X'7B7D', zeroblob(32), 'lin_test', datetime('now'))`, []any{situation, at, at, at}},
		{`INSERT INTO trigger_evaluations (
			trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane,
			outcome, reasons_json, policy_sha256, evaluated_at
		) VALUES (?, ?, ?, 'fill', ?, 1, 10, 5, 'fast', 'admitted', X'5B5D', zeroblob(32), ?)`, []any{trigger, testTenant, testSpecDigest, situation, at}},
		{`INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, lane, priority, status, dedupe_key,
			not_before, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, 1, 'fast', 10, 'pending', ?, ?, ?, datetime('now'), datetime('now'))`,
			[]any{item, trigger, testTenant, situation, dedupe[:], at, kernel.FormatTime(base.Add(15 * time.Minute))}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("seed pending item %d: %w", i, err)
		}
	}
	return nil
}
