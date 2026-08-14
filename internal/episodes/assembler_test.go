package episodes_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const testDigest = "0000000000000000000000000000000000000000000000000000000000000000"

const testSpecDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func insertSituationVersion(ctx context.Context, tx *sql.Tx, v situations.Version, deploymentID, tenantID string) error {
	snapshot := map[string]any{
		"situation_id": v.SituationID, "situation_version": v.Version,
		"situation_type": "test", "tenant_id": tenantID,
		"entity":       map[string]any{"type": v.EntityType, "id": v.EntityID},
		"partition_id": 0, "phase": v.Phase, "previous_phase": v.PreviousPhase,
		"severity": v.Severity, "confidence": v.Confidence, "completeness": v.Completeness,
		"event_horizon": v.EventHorizon.Format(time.RFC3339Nano),
		"watermark":     v.Watermark.Format(time.RFC3339Nano), "spec_digest": testSpecDigest,
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
		v.EventHorizon.Format(time.RFC3339Nano), snapshotJSON, snapshotSHA,
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
				Name:          "native",
				ModelPolicy:   "test-policy",
				PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision.",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "approval", RateLimitPerHour: 2},
				{Type: "downgrade_maintenance_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "automatic", RateLimitPerHour: 2},
				{Type: "withdraw_maintenance_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "automatic", RateLimitPerHour: 2},
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1","phase":"candidate","severity":10,"facts":{"facts.level":15}}`),
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
			compiled.Actions.WatchConfidenceFloor = &floor
			var explicitReq *episodes.Request
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				var err error
				explicitReq, err = asm.Assemble(ctx, tx, schedulerItemID, "default")
				return err
			}); err != nil {
				t.Fatalf("assemble tx: %v", err)
			}
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
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "reconsideration.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
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
	decisionJSON, err := canonicaljson.Marshal(map[string]any{
		"decision_id": "dec-prior",
		"episode_id":  "epi-prior",
		"confidence":  0.9,
		"intents":     []any{},
	})
	if err != nil {
		t.Fatalf("marshal prior decision: %v", err)
	}
	commandJSON, err := canonicaljson.Marshal(map[string]any{
		"command_id":        "cmd-prior",
		"intent_id":         "int-prior",
		"tenant_id":         "default",
		"effector_route":    "maintenance.ticket",
		"normalized_target": "motor-1",
		"idempotency_key":   "sha256:" + testDigest,
		"status":            "prepared",
		"payload":           map[string]any{"priority": "urgent"},
	})
	if err != nil {
		t.Fatalf("marshal prior command: %v", err)
	}
	deltaJSON, err := canonicaljson.Marshal(map[string]any{
		"reason":                 "prior_action_invalidated",
		"superseded_version":     1,
		"correction_version":     2,
		"invalidated_command_id": "cmd-prior",
		"prior_decision_id":      "dec-prior",
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
			) VALUES ('trg-prior', 'default', ?, 'prior', ?, 1, 10, 5, 'fast', 'admitted', ?, ?, ?, ?)`,
			testSpecDigest, prior.SituationID, []byte("[]"), zero, []byte("{}"), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, lane, priority,
				status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-prior', 'trg-prior', 'default', ?, 1, 'fast', 10, 'completed', ?, ?, ?, ?)`,
			prior.SituationID, zero, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior scheduler item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
				admission_key, request_json, lifecycle_status, current_fence, accepted_at
			) VALUES ('epi-prior', 'sch-prior', 'default', ?, 1, 'native', ?, 'test', 'v1', ?, ?, ?, 'concluded', 1, ?)`,
			prior.SituationID, testSpecDigest, zero, zero, []byte("{}"), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior episode: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO decisions (
				decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
				raw_json, decision_sha256, validation_status, validation_json, created_at
			) VALUES ('dec-prior', 'epi-prior', 'attempt-prior', 1, 1, ?, 1, ?, ?, 'accepted', ?, ?)`,
			prior.SituationID, decisionJSON, zero, []byte("{}"), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior decision: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO intents (
				intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
				intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
			) VALUES ('int-prior', 'dec-prior', 'default', ?, 1, 'maintenance.ticket', 'R1', ?, ?, ?, 'approved', ?, ?)`,
			prior.SituationID, []byte("{}"), zero, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior intent: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO commands (
				command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key,
				command_json, command_sha256, status, created_at, updated_at
			) VALUES ('cmd-prior', 'int-prior', 'default', 'maintenance.ticket', 'motor-1', ?, ?, ?, 'succeeded', ?, ?)`,
			zero, commandJSON, zero, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior command: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO outcomes (
				outcome_id, command_id, ordinal, status, provider_result_json, observed_effect_json,
				reconciliation_status, outcome_sha256, occurred_at
			) VALUES ('out-prior', 'cmd-prior', 1, 'succeeded', ?, ?, 'observed', ?, ?)`,
			[]byte(`{"accepted":true}`), []byte(`{"ticket":"T-1"}`), zero, now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert prior outcome: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO trigger_evaluations (
				trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
				score, threshold, lane, outcome, reasons_json, policy_sha256, delta_json, evaluated_at
			) VALUES ('trg-reconsider', 'default', ?, 'prior_action_invalidated', ?, 2, 100, 0, 'deep', 'admitted', ?, ?, ?, ?)`,
			testSpecDigest, correction.SituationID, []byte("[]"), zero, deltaJSON, now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reconsideration evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-reconsider', 'reconsider', 'trg-reconsider', 'default', ?, 2, 'deep', 100, 'pending', ?, ?, ?, ?)`,
			correction.SituationID, reconsiderationDedupe, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reconsideration scheduler item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reconsiderations (
				reconsideration_id, tenant_id, situation_id, superseded_version, correction_version,
				correction_snapshot_sha256, invalidated_command_id, invalidated_outcome_id,
				invalidated_outcome_sha256, trigger_id, scheduler_item_id, created_at
			) VALUES ('rec-prior', 'default', ?, 1, 2, ?, 'cmd-prior', 'out-prior', ?, 'trg-reconsider', 'sch-reconsider', ?)`,
			correction.SituationID, zero, zero, now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reconsideration: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed reconsideration: %v", err)
	}

	asm := episodes.NewAssembler(&compiled, ids.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := asm.Assemble(ctx, tx, "sch-reconsider", "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return asm.Persist(ctx, tx, req, now)
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
		if err := asm.Persist(ctx, tx, req, base); err != nil {
			return fmt.Errorf("persist: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}

	var episodeID, status string
	if err := db.QueryRowContext(ctx,
		"SELECT episode_id, lifecycle_status FROM episodes WHERE scheduler_item_id = ?", schedulerItemID,
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
	if err := db.QueryRowContext(ctx, "SELECT prompt_sha256, objective_sha256 FROM episodes WHERE episode_id = ?", episodeID).Scan(&promptSHA, &objectiveSHA); err != nil {
		t.Fatalf("query episode provenance: %v", err)
	}
	if len(promptSHA) != 32 || len(objectiveSHA) != 32 {
		t.Fatalf("invalid episode provenance lengths prompt=%d objective=%d", len(promptSHA), len(objectiveSHA))
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

func TestAssemblerMarksReconsiderationLiveEpisodeConflict(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "live-conflict.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

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
			testSpecDigest, v.SituationID, []byte("[]"), zero, []byte("{}"), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert first evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-live-first', 'standard', 'trg-live-first', 'default', ?, 1, 'deep', 10, 'admitted', ?, ?, ?, ?)`,
			v.SituationID, zero, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert first scheduler item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version,
				snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at
			) VALUES ('epi-live-first', 'sch-live-first', 'default', ?, 1, 'native', ?, '', '', ?, ?, X'7B7D', 'admitted', ?)`,
			v.SituationID, testSpecDigest, zero, zero, now.Format(time.RFC3339Nano)); err != nil {
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
			testSpecDigest, v.SituationID, []byte("[]"), zero, []byte("{}"), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reconsideration evaluation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version, lane,
				priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES ('sch-live-reconsider', 'reconsider', 'trg-live-reconsider', 'default', ?, 1, 'deep', 100, 'pending', ?, ?, ?, ?)`,
			v.SituationID, dedupe, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reconsideration scheduler item: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed reconsideration item: %v", err)
	}

	asm := episodes.NewAssembler(&compiled, ids.Deterministic())
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return asm.Persist(ctx, tx, &episodes.Request{
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
	if !errors.Is(err, episodes.ErrLiveEpisodeConflict) {
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1"}`),
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
			Executor: spec.Executor{Name: "native"},
		},
		Actions: spec.Actions{},
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
		SnapshotJSON:   []byte(`{"situation_id":"sit-1"}`),
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
		if err := asm.Persist(ctx, tx, req, base); err != nil {
			return fmt.Errorf("first persist: %w", err)
		}
		// Second persist should fail because scheduler item is no longer pending.
		if err := asm.Persist(ctx, tx, req, base); err == nil {
			return fmt.Errorf("expected error persisting non-pending item")
		} else if errors.Is(err, episodes.ErrLiveEpisodeConflict) {
			return fmt.Errorf("non-constraint storage error was classified as live-episode conflict: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("persist tx: %v", err)
	}
}
