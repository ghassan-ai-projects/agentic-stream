package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPipelineCompletesDecisionToSimulatedOutcome(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs: []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:   spec.TimePolicy{MaxOutOfOrderness: "1m"}, Windows: []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}}, Executor: spec.Executor{Name: "native", ModelPolicy: "test", PromptVersion: "v1", DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}}},
		Actions:   spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "automatic"}}},
	}
	path := filepath.Join(t.TempDir(), "event.jsonl")
	trace := `{"id":"evt-e2e","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":15}}` + "\n"
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(), Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector()})
	if err != nil {
		t.Fatal(err)
	}
	report, err := pipeline.RunJSONL(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.EpisodesExecuted != 1 || report.IntentsEvaluated != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("unexpected end-to-end report: %+v", report)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM commands LIMIT 1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("command status=%q", status)
	}
}
