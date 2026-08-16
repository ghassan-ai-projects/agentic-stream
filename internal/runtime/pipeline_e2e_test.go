package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
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
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}}, Executor: spec.Executor{Name: "native", DispatchPolicy: "active", ModelPolicy: "test", PromptVersion: "v1", DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}}},
		Actions:   spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: runtimeTicketSchema(), Policy: "automatic"}}},
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

func TestPipelineCorrectsLateWindowAndAdmitsOneReconsideration(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "late-correction.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs:    []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s", AllowedLateness: "5m", LatePolicy: "correct_and_reconsider"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "5m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast", MaterialDelta: "delta.phase_changed"}}, Executor: spec.Executor{Name: "native", DispatchPolicy: "active", ModelPolicy: "test", PromptVersion: "v1", DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}}},
		Actions:   spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: runtimeTicketSchema(), Policy: "automatic"}}},
	}
	base := "2026-08-12T00:00:00Z"
	firstPath := filepath.Join(t.TempDir(), "first.jsonl")
	if err := os.WriteFile(firstPath, []byte(`{"id":"evt-first","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"`+base+`","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":15}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(t.TempDir(), "late.jsonl")
	if err := os.WriteFile(secondPath, []byte(`{"id":"evt-late","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-11T23:59:00Z","ingested_at":"2026-08-12T00:00:02Z","classification":"internal","data":{"level":20}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(), Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector()})
	if err != nil {
		t.Fatal(err)
	}
	firstReport, err := pipeline.RunJSONL(ctx, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.EpisodesExecuted != 1 || firstReport.CommandsDispatched != 1 {
		t.Fatalf("first batch did not complete the prior action: %+v", firstReport)
	}
	secondLog := eventlog.NewEventLogWithClock(db, clock.Physical())
	secondReplay := ingress.NewJSONLReplay(db, secondLog, "default", secondPath, "live-jsonl:"+secondPath)
	if ingested, err := secondReplay.Run(ctx); err != nil {
		t.Fatal(err)
	} else if ingested != 1 {
		t.Fatalf("second batch ingested %d events, want 1", ingested)
	}
	stream, err := engine.NewEngine(ctx, db, secondLog, clock.Physical(), compiled, "default")
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := stream.RunGlobal(ctx, nil); err != nil {
		t.Fatal(err)
	} else if processed < 1 {
		t.Fatalf("second batch processed %d events, want at least 1", processed)
	}
	if ingested, err := secondReplay.Run(ctx); err != nil {
		t.Fatal(err)
	} else if ingested != 0 {
		t.Fatalf("replayed late batch ingested %d events, want 0", ingested)
	}
	if _, err := stream.RunGlobal(ctx, nil); err != nil {
		t.Fatal(err)
	}
	var correctedVersions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM situation_versions WHERE completeness = 'corrected'").Scan(&correctedVersions); err != nil {
		t.Fatal(err)
	}
	if correctedVersions != 1 {
		t.Fatalf("corrected situation versions = %d, want 1", correctedVersions)
	}
	var reconsiderations, reconsiderationNotifications int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'").Scan(&reconsiderations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE event_type = 'io.agenticstream.reconsideration.admitted.v1'").Scan(&reconsiderationNotifications); err != nil {
		t.Fatal(err)
	}
	if reconsiderations != 1 || reconsiderationNotifications != 1 {
		t.Fatalf("reconsideration admissions = %d, notifications = %d, want one each", reconsiderations, reconsiderationNotifications)
	}
}
