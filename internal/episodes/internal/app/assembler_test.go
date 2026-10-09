package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

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

func TestAssemblerBuildsEpisodeRequest(t *testing.T) {
	ctx := context.Background()
	compiled := triggeredSpec(
		spec.Executor{Name: "native", ModelPolicy: "test-policy", PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision."},
		spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema(), Policy: "approval", RateLimitPerHour: 2},
		spec.Intent{Type: "downgrade_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema(), Policy: "automatic", RateLimitPerHour: 2},
		spec.Intent{Type: "withdraw_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema(), Policy: "automatic", RateLimitPerHour: 2},
	)
	s := admitTriggeredSituation(t, ctx, compiled, "sit-1")
	req := s.assemble(t, ctx)

	if req.EpisodeID == "" {
		t.Fatal("expected episode id")
	}
	if req.SchedulerItemID != s.schedulerItemID {
		t.Fatalf("expected scheduler item id %s, got %s", s.schedulerItemID, req.SchedulerItemID)
	}
	if req.SituationID != s.version.SituationID {
		t.Fatalf("expected situation id %s, got %s", s.version.SituationID, req.SituationID)
	}
	if req.ExecutorName != "native" {
		t.Fatalf("expected executor native, got %s", req.ExecutorName)
	}
	if req.SnapshotSHA256 == "" {
		t.Fatal("expected snapshot sha256")
	}
	if req.PromptSHA256 == "" || req.ObjectiveSHA256 == "" {
		t.Fatalf("expected prompt/objective provenance digests, got prompt=%q objective=%q", req.PromptSHA256, req.ObjectiveSHA256)
	}
	if len(req.AdmissionKey) != 32 {
		t.Fatalf("expected admission key length 32, got %d", len(req.AdmissionKey))
	}
	if len(req.RequestJSON) == 0 {
		t.Fatal("expected request json")
	}
	var payload struct {
		AllowedIntentTypes   []string `json:"allowed_intent_types"`
		WatchConfidenceFloor float64  `json:"watch_confidence_floor"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	wantAllowed := []string{"create_ticket", "downgrade_maintenance_ticket", "withdraw_maintenance_ticket"}
	if !slices.Equal(payload.AllowedIntentTypes, wantAllowed) {
		t.Fatalf("allowed intent types = %v, want %v", payload.AllowedIntentTypes, wantAllowed)
	}
	if payload.WatchConfidenceFloor != 0.5 {
		t.Fatalf("watch confidence floor = %v, want 0.5", payload.WatchConfidenceFloor)
	}

	for _, tt := range []struct {
		name  string
		floor float64
	}{
		{name: "custom", floor: 0.7},
		{name: "opt out", floor: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			floor := tt.floor
			s.compiled.Actions.WatchConfidenceFloor = &floor
			explicitReq := s.assemble(t, ctx)
			var explicitPayload struct {
				WatchConfidenceFloor float64 `json:"watch_confidence_floor"`
			}
			if err := json.Unmarshal(explicitReq.RequestJSON, &explicitPayload); err != nil {
				t.Fatalf("unmarshal explicit request: %v", err)
			}
			if explicitPayload.WatchConfidenceFloor != tt.floor {
				t.Fatalf("watch confidence floor = %v, want %v", explicitPayload.WatchConfidenceFloor, tt.floor)
			}
		})
	}
}

func TestAssemblerPersistsReconsiderationPayload(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	db.SetMaxOpenConns(1)

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testSpecDigest,
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
		},
		Cognition: spec.Cognition{
			Executor: spec.Executor{Name: "tamoz", ModelPolicy: "test", PromptVersion: "v1"},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
			},
		},
	}
	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}

	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	prior := situations.Version{
		SituationID:  "sit-reconsider",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1,
		Completeness: "on_time",
		EntityType:   "motor",
		EntityID:     "motor-1",
		EventHorizon: now,
		Watermark:    now,
		Facts:        map[string]any{"level": 15},
	}
	correction := situations.Version{
		SituationID:  prior.SituationID,
		Version:      2,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1,
		Completeness: "corrected",
		EntityType:   prior.EntityType,
		EntityID:     prior.EntityID,
		EventHorizon: now,
		Watermark:    now,
		Facts:        map[string]any{"level": 20},
	}
	zero := make([]byte, 32)
	reconsiderationDedupe := make([]byte, 32)
	reconsiderationDedupe[0] = 1
	deltaJSON, err := canonicaljson.Marshal(map[string]any{
		"reason":                 "prior_action_invalidated",
		"superseded_version":     1,
		"correction_version":     2,
		"invalidated_command_id": "cmd-prior",
		"prior_decision_id":      "dec-prior",
		"reconsideration_id":     "rec-prior",
		"invalidated_outcome_id": "out-prior",
		"prior_decision":         map[string]any{"decision_id": "dec-prior", "decision_type": "need_more_evidence"},
		"prior_command":          map[string]any{"command_id": "cmd-prior", "status": "succeeded"},
		"prior_outcome":          map[string]any{"outcome_id": "out-prior", "command_id": "cmd-prior", "status": "succeeded"},
		"correction": map[string]any{
			"situation_id":      prior.SituationID,
			"situation_version": 2,
		},
	})
	if err != nil {
		t.Fatalf("marshal reconsideration delta: %v", err)
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, prior, testSpecDigest, "default"); err != nil {
			return err
		}
		if err := insertSituationVersion(ctx, tx, correction, testSpecDigest, "default"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO trigger_evaluations (
				trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
				score, threshold, lane, outcome, reasons_json, policy_sha256, delta_json, evaluated_at
			) VALUES ('trg-reconsider', 'default', ?, 'prior_action_invalidated', ?, 2, 100, 0, 'deep', 'admitted', ?, ?, ?, ?)`,
			testSpecDigest, correction.SituationID, []byte("[]"), zero, deltaJSON, kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert reconsideration evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-reconsider', 'reconsider', 'trg-reconsider', 'default', ?, 2, 'deep', 100, 'pending', ?, ?, ?, ?)`,
			correction.SituationID, reconsiderationDedupe, kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert reconsideration scheduler item: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed reconsideration: %v", err)
	}

	asm := app.NewAssembler(&compiled, sources.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := asm.Assemble(ctx, store.Join(tx), "sch-reconsider", "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return asm.Persist(ctx, store.Join(tx), req, now)
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}

	var requestJSON []byte
	if err := db.QueryRowContext(ctx, "SELECT request_json FROM episodes WHERE scheduler_item_id = ?", "sch-reconsider").Scan(&requestJSON); err != nil {
		t.Fatalf("load persisted request: %v", err)
	}
	var payload struct {
		Kind            string `json:"kind"`
		Reconsideration struct {
			PriorDecision map[string]any   `json:"prior_decision"`
			Commands      []map[string]any `json:"commands"`
			Outcomes      []map[string]any `json:"outcomes"`
			Correction    map[string]any   `json:"correction"`
		} `json:"reconsideration"`
	}
	if err := json.Unmarshal(requestJSON, &payload); err != nil {
		t.Fatalf("decode persisted request: %v", err)
	}
	if payload.Kind != "reconsider" {
		t.Fatalf("request kind = %q, want reconsider", payload.Kind)
	}
	if got := payload.Reconsideration.PriorDecision["decision_id"]; got != "dec-prior" {
		t.Fatalf("prior decision id = %v, want dec-prior", got)
	}
	if len(payload.Reconsideration.Commands) != 1 || payload.Reconsideration.Commands[0]["command_id"] != "cmd-prior" {
		t.Fatalf("reconsideration commands = %#v", payload.Reconsideration.Commands)
	}
	if len(payload.Reconsideration.Outcomes) != 1 || payload.Reconsideration.Outcomes[0]["outcome_id"] != "out-prior" {
		t.Fatalf("reconsideration outcomes = %#v", payload.Reconsideration.Outcomes)
	}
	if invalidates, ok := payload.Reconsideration.Correction["invalidates"].([]any); !ok || len(invalidates) != 1 || invalidates[0] != "cmd-prior" {
		t.Fatalf("reconsideration correction invalidates = %#v", payload.Reconsideration.Correction["invalidates"])
	}
}

func TestAssemblerPersistCreatesEpisode(t *testing.T) {
	ctx := context.Background()
	compiled := triggeredSpec(
		spec.Executor{Name: "native", ModelPolicy: "test-policy", PromptVersion: "prompt-v1"},
		spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	)
	s := admitTriggeredSituation(t, ctx, compiled, "sit-1")
	s.assembleAndPersist(t, ctx)

	var episodeID, status string
	if err := s.db.QueryRowContext(ctx,
		"SELECT episode_id, lifecycle_status FROM episodes WHERE scheduler_item_id = ?", s.schedulerItemID,
	).Scan(&episodeID, &status); err != nil {
		t.Fatalf("query episode: %v", err)
	}
	if episodeID == "" {
		t.Fatal("expected episode id")
	}
	if status != "admitted" {
		t.Fatalf("expected admitted lifecycle status, got %s", status)
	}
	var promptSHA, objectiveSHA []byte
	if err := s.db.QueryRowContext(ctx, "SELECT prompt_sha256, objective_sha256 FROM episodes WHERE episode_id = ?", episodeID).Scan(&promptSHA, &objectiveSHA); err != nil {
		t.Fatalf("query episode provenance: %v", err)
	}
	if len(promptSHA) != 32 || len(objectiveSHA) != 32 {
		t.Fatalf("invalid episode provenance lengths prompt=%d objective=%d", len(promptSHA), len(objectiveSHA))
	}

	var itemStatus string
	if err := s.db.QueryRowContext(ctx,
		"SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", s.schedulerItemID,
	).Scan(&itemStatus); err != nil {
		t.Fatalf("query scheduler item status: %v", err)
	}
	if itemStatus != "admitted" {
		t.Fatalf("expected admitted scheduler item, got %s", itemStatus)
	}
}

func TestAssemblerMarksReconsiderationLiveEpisodeConflict(t *testing.T) {
	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testSpecDigest,
		Situation: spec.Situation{
			Type:   "test",
			Phases: []spec.Phase{{Name: "candidate", Severity: 10}},
		},
		Cognition: spec.Cognition{Executor: spec.Executor{Name: "native"}},
	}
	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	v := situations.Version{
		SituationID:  "sit-live-conflict",
		Version:      1,
		Phase:        "candidate",
		Severity:     10,
		Confidence:   1,
		Completeness: "on_time",
		EntityType:   "motor",
		EntityID:     "motor-1",
		EventHorizon: now,
		Watermark:    now,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v, testSpecDigest, "default"); err != nil {
			return err
		}
		zero := make([]byte, 32)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO trigger_evaluations (
				trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
				score, threshold, lane, outcome, reasons_json, policy_sha256, delta_json, evaluated_at
			) VALUES ('trg-live-first', 'default', ?, 'first', ?, 1, 10, 0, 'deep', 'admitted', ?, ?, ?, ?)`,
			testSpecDigest, v.SituationID, []byte("[]"), zero, []byte("{}"), kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert first evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-live-first', 'standard', 'trg-live-first', 'default', ?, 1, 'deep', 10, 'admitted', ?, ?, ?, ?)`,
			v.SituationID, zero, kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert first scheduler item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version,
				snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at
			) VALUES ('epi-live-first', 'sch-live-first', 'default', ?, 1, 'native', ?, '', '', ?, ?, X'7B7D', 'admitted', ?)`,
			v.SituationID, testSpecDigest, zero, zero, kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert live episode: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed live episode: %v", err)
	}

	zero := make([]byte, 32)
	dedupe := make([]byte, 32)
	dedupe[0] = 1
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO trigger_evaluations (
				trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
				score, threshold, lane, outcome, reasons_json, policy_sha256, delta_json, evaluated_at
			) VALUES ('trg-live-reconsider', 'default', ?, 'prior_action_invalidated', ?, 1, 100, 0, 'deep', 'admitted', ?, ?, ?, ?)`,
			testSpecDigest, v.SituationID, []byte("[]"), zero, []byte("{}"), kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert reconsideration evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-live-reconsider', 'reconsider', 'trg-live-reconsider', 'default', ?, 1, 'deep', 100, 'pending', ?, ?, ?, ?)`,
			v.SituationID, dedupe, kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("insert reconsideration scheduler item: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed reconsideration item: %v", err)
	}

	asm := app.NewAssembler(&compiled, sources.Deterministic())
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return asm.Persist(ctx, store.Join(tx), &app.Request{
			EpisodeID:        "epi-live-second",
			SchedulerItemID:  "sch-live-reconsider",
			Kind:             "reconsider",
			TenantID:         "default",
			SituationID:      v.SituationID,
			SituationVersion: 1,
			ExecutorName:     "native",
			ExecutorVersion:  testSpecDigest,
			SnapshotSHA256:   testSpecDigest,
			PromptSHA256:     testSpecDigest,
			ObjectiveSHA256:  testSpecDigest,
			AdmissionKey:     append([]byte{2}, make([]byte, 31)...),
			RequestJSON:      []byte("{}"),
		}, now)
	})
	if !errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
		t.Fatalf("persist error = %v, want ErrLiveEpisodeConflict", err)
	}
	var episodesCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episodes WHERE situation_id = ?", v.SituationID).Scan(&episodesCount); err != nil {
		t.Fatalf("count episodes: %v", err)
	}
	if episodesCount != 1 {
		t.Fatalf("episodes after conflict = %d, want 1", episodesCount)
	}
}

func TestAssemblerIsDeterministic(t *testing.T) {
	ctx := context.Background()
	compiled := triggeredSpec(
		spec.Executor{Name: "native", ModelPolicy: "test-policy", PromptVersion: "prompt-v1"},
		spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	)
	s := admitTriggeredSituation(t, ctx, compiled, "sit-1")

	req1 := s.assemble(t, ctx)
	req2 := s.assemble(t, ctx)

	if req1.SnapshotSHA256 != req2.SnapshotSHA256 {
		t.Fatalf("deterministic snapshot hash mismatch: %s vs %s", req1.SnapshotSHA256, req2.SnapshotSHA256)
	}
}

func TestAssemblerRequestContainsDelta(t *testing.T) {
	ctx := context.Background()
	s := admitTriggeredSituation(t, ctx, triggeredSpec(spec.Executor{Name: "native"}, spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()}), "sit-1")

	req := s.assemble(t, ctx)

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
	s := admitTriggeredSituation(t, ctx, triggeredSpec(spec.Executor{Name: "native"}, spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()}), "sit-1")

	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := s.asm.Assemble(ctx, store.Join(tx), s.schedulerItemID, "other-tenant")
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
	s := admitTriggeredSituation(t, ctx, triggeredSpec(spec.Executor{Name: "native"}, spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()}), "sit-1")

	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := s.asm.Assemble(ctx, store.Join(tx), s.schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		if err := s.asm.Persist(ctx, store.Join(tx), req, s.base); err != nil {
			return fmt.Errorf("first persist: %w", err)
		}
		// Second persist should fail because scheduler item is no longer pending.
		if err := s.asm.Persist(ctx, store.Join(tx), req, s.base); err == nil {
			return fmt.Errorf("expected error persisting non-pending item")
		} else if errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
			return fmt.Errorf("non-constraint storage error was classified as live-episode conflict: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("persist tx: %v", err)
	}
}

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}
