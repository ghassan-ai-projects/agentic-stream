package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var reconsiderationNow = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

type schedulerItemSeed struct {
	itemID, triggerID, triggerName, kind, status string
	situationID                                  string
	version, score, priority                     int
	deltaJSON                                    []byte
	dedupeFirstByte                              byte
}

func (seed schedulerItemSeed) insert(ctx context.Context, tx *sql.Tx) error {
	zero := make([]byte, 32)
	dedupe := make([]byte, 32)
	dedupe[0] = seed.dedupeFirstByte
	now := kernel.FormatTime(reconsiderationNow)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO trigger_evaluations (
			trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
			score, threshold, lane, outcome, reasons_json, policy_sha256, delta_json, evaluated_at
		) VALUES (?, 'default', ?, ?, ?, ?, ?, 0, 'deep', 'admitted', X'5B5D', ?, ?, ?)`,
		seed.triggerID, testSpecDigest, seed.triggerName, seed.situationID, seed.version, seed.score, zero, seed.deltaJSON, now); err != nil {
		return fmt.Errorf("insert evaluation %s: %w", seed.triggerID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scheduler_items (
			scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
			priority, status, dedupe_key, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, 'default', ?, ?, 'deep', ?, ?, ?, ?, ?, ?)`,
		seed.itemID, seed.kind, seed.triggerID, seed.situationID, seed.version, seed.priority, seed.status, dedupe,
		kernel.FormatTime(reconsiderationNow.Add(time.Hour)), now, now); err != nil {
		return fmt.Errorf("insert scheduler item %s: %w", seed.itemID, err)
	}
	return nil
}

func situationVersion(situationID string, version int, completeness string, facts map[string]any) situations.Version {
	return situations.Version{
		SituationID: situationID, Version: version, Phase: "candidate", Severity: 10, Confidence: 1,
		Completeness: completeness, EntityType: "motor", EntityID: "motor-1",
		EventHorizon: reconsiderationNow, Watermark: reconsiderationNow, Facts: facts,
	}
}

func deployedSpec(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec) {
	t.Helper()
	if err := spec.SaveDeployment(t.Context(), db, "default", compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
}

func seedInTx(t *testing.T, db *storage.DB, work func(ctx context.Context, tx *sql.Tx) error) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return work(t.Context(), tx) }); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestAssemblerBuildsTheReconsiderationFromThePriorActionEvidence(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: testSpecDigest,
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}},
		Cognition: spec.Cognition{Executor: spec.Executor{Name: "tamoz", ModelPolicy: "test", PromptVersion: "v1"}},
		Actions:   spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()}}},
	}
	deployedSpec(t, db, compiled)
	delta, err := canonicaljson.Marshal(map[string]any{
		"reason": "prior_action_invalidated", "superseded_version": 1, "correction_version": 2,
		"invalidated_command_id": "cmd-prior", "prior_decision_id": "dec-prior", "reconsideration_id": "rec-prior",
		"invalidated_outcome_id": "out-prior",
		"prior_decision":         map[string]any{"decision_id": "dec-prior", "decision_type": "need_more_evidence"},
		"prior_command":          map[string]any{"command_id": "cmd-prior", "status": "succeeded"},
		"prior_outcome":          map[string]any{"outcome_id": "out-prior", "command_id": "cmd-prior", "status": "succeeded"},
		"correction":             map[string]any{"situation_id": "sit-reconsider", "situation_version": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedInTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		for _, version := range []situations.Version{
			situationVersion("sit-reconsider", 1, "on_time", map[string]any{"level": 15}),
			situationVersion("sit-reconsider", 2, "corrected", map[string]any{"level": 20}),
		} {
			if err := insertSituationVersion(ctx, tx, version, testSpecDigest, "default"); err != nil {
				return err
			}
		}
		return schedulerItemSeed{itemID: "sch-reconsider", triggerID: "trg-reconsider", triggerName: "prior_action_invalidated", kind: "reconsider",
			status: "pending", situationID: "sit-reconsider", version: 2, score: 100, priority: 100, deltaJSON: delta, dedupeFirstByte: 1}.insert(ctx, tx)
	})
	asm := app.NewAssembler(compiled, sources.Deterministic())
	seedInTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		req, err := asm.Assemble(ctx, store.Join(tx), "sch-reconsider", "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return asm.Persist(ctx, store.Join(tx), req, reconsiderationNow)
	})

	var payload struct {
		Kind            string `json:"kind"`
		Reconsideration struct {
			PriorDecision map[string]any   `json:"prior_decision"`
			Commands      []map[string]any `json:"commands"`
			Outcomes      []map[string]any `json:"outcomes"`
			Correction    map[string]any   `json:"correction"`
		} `json:"reconsideration"`
	}
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT request_json FROM episodes WHERE scheduler_item_id = 'sch-reconsider'"), &payload); err != nil {
		t.Fatalf("decode persisted request: %v", err)
	}
	reconsideration := payload.Reconsideration
	invalidates, _ := reconsideration.Correction["invalidates"].([]any)
	if payload.Kind != "reconsider" || reconsideration.PriorDecision["decision_id"] != "dec-prior" ||
		len(reconsideration.Commands) != 1 || reconsideration.Commands[0]["command_id"] != "cmd-prior" ||
		len(reconsideration.Outcomes) != 1 || reconsideration.Outcomes[0]["outcome_id"] != "out-prior" ||
		len(invalidates) != 1 || invalidates[0] != "cmd-prior" {
		t.Fatalf("persisted reconsideration = kind %q %+v", payload.Kind, reconsideration)
	}
}

func TestPersistRefusesASecondLiveEpisodeForTheSameSituation(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: testSpecDigest,
		Situation: spec.Situation{Type: "test", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}},
		Cognition: spec.Cognition{Executor: spec.Executor{Name: "native"}},
	}
	deployedSpec(t, db, compiled)
	seedInTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, situationVersion("sit-live", 1, "on_time", nil), testSpecDigest, "default"); err != nil {
			return err
		}
		if err := (schedulerItemSeed{itemID: "sch-first", triggerID: "trg-first", triggerName: "first", kind: "standard", status: "admitted",
			situationID: "sit-live", version: 1, score: 10, priority: 10, deltaJSON: []byte("{}")}).insert(ctx, tx); err != nil {
			return err
		}
		zero := make([]byte, 32)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version,
				snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at
			) VALUES ('epi-live-first', 'sch-first', 'default', 'sit-live', 1, 'native', ?, '', '', ?, ?, X'7B7D', 'admitted', ?)`,
			testSpecDigest, zero, zero, kernel.FormatTime(reconsiderationNow)); err != nil {
			return fmt.Errorf("insert live episode: %w", err)
		}
		return schedulerItemSeed{itemID: "sch-second", triggerID: "trg-second", triggerName: "prior_action_invalidated", kind: "reconsider",
			status: "pending", situationID: "sit-live", version: 1, score: 100, priority: 100, deltaJSON: []byte("{}"), dedupeFirstByte: 1}.insert(ctx, tx)
	})
	asm := app.NewAssembler(compiled, sources.Deterministic())
	second := &app.Request{
		EpisodeID: "epi-live-second", SchedulerItemID: "sch-second", Kind: "reconsider", TenantID: "default",
		SituationID: "sit-live", SituationVersion: 1, ExecutorName: "native", ExecutorVersion: testSpecDigest,
		SnapshotSHA256: testSpecDigest, PromptSHA256: testSpecDigest, ObjectiveSHA256: testSpecDigest,
		AdmissionKey: append([]byte{2}, make([]byte, 31)...), RequestJSON: []byte("{}"),
	}

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return asm.Persist(t.Context(), store.Join(tx), second, reconsiderationNow) })

	if !errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
		t.Fatalf("persist error = %v, want ErrLiveEpisodeConflict", err)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM episodes WHERE situation_id = 'sit-live'"); got != 1 {
		t.Fatalf("episodes after conflict = %d, want 1", got)
	}
}
