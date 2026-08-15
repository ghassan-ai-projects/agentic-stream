package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPipelineSkipsSecondReconsiderationForOneSituation(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "reconsideration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs: []spec.Input{{
			Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal",
		}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s", AllowedLateness: "5m", LatePolicy: "correct_and_reconsider"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "5m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{
			Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}},
			Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"},
			Reducers:   []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast", MaterialDelta: "delta.phase_changed"}},
			Executor: spec.Executor{Name: "native", ModelPolicy: "test", PromptVersion: "v1", DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}},
		},
		Actions: spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: runtimeTicketSchema(), Policy: "automatic"}}},
	}

	firstPath := filepath.Join(t.TempDir(), "first.jsonl")
	if err := os.WriteFile(firstPath, []byte(`{"id":"evt-first","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":15}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	latePath := filepath.Join(t.TempDir(), "late.jsonl")
	if err := os.WriteFile(latePath, []byte(`{"id":"evt-late","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-11T23:59:00Z","ingested_at":"2026-08-12T00:00:02Z","classification":"internal","data":{"level":20}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pipeline, err := NewPipeline(ctx, PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(),
		Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunJSONL(ctx, firstPath); err != nil {
		t.Fatalf("first pipeline batch: %v", err)
	}
	if err := insertSecondInvalidatedCommand(ctx, db); err != nil {
		t.Fatalf("insert second invalidated command: %v", err)
	}

	replay := ingress.NewJSONLReplay(db, pipeline.log, "default", latePath, "live-jsonl:"+latePath)
	if count, err := replay.Run(ctx); err != nil {
		t.Fatalf("ingest correction: %v", err)
	} else if count != 1 {
		t.Fatalf("correction events ingested = %d, want 1", count)
	}
	if _, err := pipeline.engine.RunGlobal(ctx, nil); err != nil {
		t.Fatalf("process correction: %v", err)
	}
	if admitted, err := pipeline.assemblePending(ctx); err != nil {
		t.Fatalf("assemble reconsiderations: %v", err)
	} else if admitted != 1 {
		t.Fatalf("episodes admitted = %d, want 1", admitted)
	}

	var live int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM episodes e JOIN scheduler_items si ON si.scheduler_item_id = e.scheduler_item_id
		WHERE si.kind = 'reconsider' AND e.lifecycle_status IN ('admitted', 'running')`).Scan(&live); err != nil {
		t.Fatalf("count live reconsideration episodes: %v", err)
	}
	if live != 1 {
		t.Fatalf("live reconsideration episodes = %d, want 1", live)
	}
	var skipped int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider' AND status = 'coalesced'").Scan(&skipped); err != nil {
		t.Fatalf("count skipped reconsideration items: %v", err)
	}
	if skipped != 1 {
		t.Fatalf("coalesced reconsideration items = %d, want 1", skipped)
	}
	var reconsiderations int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reconsiderations").Scan(&reconsiderations); err != nil {
		t.Fatalf("count reconsiderations: %v", err)
	}
	if reconsiderations != 2 {
		t.Fatalf("reconsiderations = %d, want 2", reconsiderations)
	}
}

func insertSecondInvalidatedCommand(ctx context.Context, db *storage.DB) error {
	var decisionID, situationID string
	var situationVersion int
	if err := db.QueryRowContext(ctx, `
		SELECT d.decision_id, i.situation_id, i.situation_version
		FROM decisions d JOIN intents i ON i.decision_id = d.decision_id
		WHERE d.validation_status = 'accepted' AND i.policy_status = 'approved'
		ORDER BY d.created_at, d.decision_id LIMIT 1`).Scan(&decisionID, &situationID, &situationVersion); err != nil {
		return err
	}
	now := "2026-08-12T00:00:03Z"
	zero := make([]byte, 32)
	idempotencyKey := make([]byte, 32)
	idempotencyKey[0] = 2
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO intents (
				intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
				intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
			) VALUES ('int-invalidated-second', ?, 'default', ?, ?, 'create_maintenance_ticket', 'R1', X'7B7D', ?, ?, 'approved', ?, ?)`,
			decisionID, situationID, situationVersion, zero, "2099-01-01T00:00:00Z", now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO commands (
				command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json,
				command_sha256, status, created_at, updated_at
			) VALUES ('cmd-invalidated-second', 'int-invalidated-second', 'default', 'maintenance.ticket', 'motor-1', ?, X'7B7D', ?, 'succeeded', ?, ?)`,
			idempotencyKey, zero, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO outcomes (
				outcome_id, command_id, ordinal, status, reconciliation_status, outcome_sha256, occurred_at
			) VALUES ('outcome-invalidated-second', 'cmd-invalidated-second', 1, 'succeeded', 'observed', ?, ?)`,
			zero, now); err != nil {
			return err
		}
		return nil
	})
}

func runtimeTicketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}
