package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"

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
	if executor.RiskCeiling == "" {
		executor.RiskCeiling = "R1"
	}
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

func admitTriggeredSituation(t *testing.T, compiled *spec.CompiledSpec, situationID string) *admittedSituation {
	t.Helper()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)
	if err := spec.SaveDeployment(ctx, db, "default", compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	base := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	eng, err := cognition.New(cognition.Config{DeploymentID: testSpecDigest, TenantID: "default", Spec: compiled, IDGen: sources.Deterministic(), Clock: sources.NewVirtual(base)})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
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

func (s *admittedSituation) assemble(t *testing.T) *app.Request {
	t.Helper()
	ctx := t.Context()
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

func (s *admittedSituation) assembleAndPersist(t *testing.T) {
	t.Helper()
	ctx := t.Context()
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

const testSpecDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func insertSituationVersion(ctx context.Context, tx *sql.Tx, v situations.Version, deploymentID, tenantID string) error {
	snapshot := map[string]any{
		"situation_id": v.SituationID, "situation_version": v.Version,
		"situation_type": "test", "tenant_id": tenantID,
		"entity":       map[string]any{"type": v.EntityType, "id": v.EntityID},
		"partition_id": 0, "phase": v.Phase, "previous_phase": v.PreviousPhase,
		"severity": v.Severity, "confidence": v.Confidence, "completeness": v.Completeness,
		"event_horizon": kernel.FormatTime(v.EventHorizon),
		"watermark":     kernel.FormatTime(v.Watermark), "spec_digest": testSpecDigest,
		"facts": v.Facts, "evidence": []any{},
	}
	snapshotJSON, err := canonicaljson.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal test snapshot: %w", err)
	}
	snapshotDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return fmt.Errorf("digest test snapshot: %w", err)
	}
	snapshotSHA, err := canonicaljson.DecodeDigest(snapshotDigest)
	if err != nil {
		return fmt.Errorf("decode test snapshot digest: %w", err)
	}
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
		kernel.FormatTime(v.EventHorizon), kernel.FormatTime(v.EventHorizon),
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
		kernel.FormatTime(v.EventHorizon), kernel.FormatTime(v.Watermark),
		kernel.FormatTime(v.EventHorizon), snapshotJSON, snapshotSHA,
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}
