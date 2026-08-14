package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPipelineKeepsIngestingWhenCostReservationIsRejected(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cost-rejection.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs: []spec.Input{{
			Name: "level", EventType: "station.observed", SchemaVersion: "1.0",
			PartitionKey: "entity.id", EntityType: "station", Classification: "internal",
		}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{
			Type: "station_alarm", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}},
			Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"},
			Reducers:   []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}},
			Executor: spec.Executor{
				Name: "native", ModelPolicy: "test", PromptVersion: "v1", DecisionSchema: "schemas/decision.json",
				Budget: spec.Budget{WallTime: "5s", CostMicrounits: 2},
			},
		},
		Actions: spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", Schema: "schemas/ticket.json", Policy: "automatic"}}},
	}

	tracePath := filepath.Join(t.TempDir(), "stations.jsonl")
	trace := `{"id":"station-a-001","type":"station.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"station-a","entity":{"type":"station","id":"station-a"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":15}}
{"id":"station-b-001","type":"station.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"station-b","entity":{"type":"station","id":"station-b"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:02Z","classification":"internal","data":{"level":15}}
`
	if err := os.WriteFile(tracePath, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}

	ceiling := uint64(1)
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(),
		GlobalCostCeiling: &ceiling, Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := pipeline.RunJSONL(ctx, tracePath)
	if err != nil {
		t.Fatalf("cost rejection stopped pipeline: %v", err)
	}
	if report.EventsIngested != 2 || report.EventsProcessed != 2 || report.EpisodesAdmitted != 0 {
		t.Fatalf("unexpected cost-rejection report: %+v", report)
	}

	var eventCount, episodeCount, skippedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_log WHERE tenant_id = 'default'").Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episodes WHERE tenant_id = 'default'").Scan(&episodeCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_items WHERE status = 'coalesced'").Scan(&skippedCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 || episodeCount != 0 || skippedCount != 2 {
		t.Fatalf("event count=%d, episode count=%d, skipped scheduler items=%d; want 2, 0, 2", eventCount, episodeCount, skippedCount)
	}

	var reasons string
	if err := db.QueryRowContext(ctx, `
		SELECT CAST(te.reasons_json AS TEXT)
		FROM trigger_evaluations te JOIN scheduler_items si ON si.trigger_id = te.trigger_id
		ORDER BY si.scheduler_item_id LIMIT 1`).Scan(&reasons); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reasons, "cost ceiling or kill switch rejected") {
		t.Fatalf("cost rejection reason = %q", reasons)
	}
}
