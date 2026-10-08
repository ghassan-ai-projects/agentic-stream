package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func modeCompiledSpec(executorName, dispatchPolicy string) *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs:    []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}},
			Executor: spec.Executor{Name: executorName, DispatchPolicy: dispatchPolicy, ModelPolicy: "test", PromptVersion: "v1",
				DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}}},
		Actions: spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1",
			ParameterSchema: runtimeTicketSchema(), Policy: "automatic"}}},
	}
}

func modeTraceFile(t *testing.T, level int) string {
	return modeTraceForEntity(t, level, "evt-p8", "ent-1")
}

func modeTraceForEntity(t *testing.T, level int, eventID, entityID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p8-event.jsonl")
	trace := `{"id":"evt-p8","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":` + fmt.Sprint(level) + `}}` + "\n"
	trace = strings.NewReplacer("evt-p8", eventID, "ent-1", entityID).Replace(trace)
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func openModeDB(t *testing.T, name string) *storage.DB {
	t.Helper()
	db, err := storagetest.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newModePipeline(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, cfg runtime.PipelineConfig) *runtime.Pipeline {
	t.Helper()
	if cfg.OwnerEpoch != "" {
		owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: cfg.OwnerEpoch + "-instance", Lease: 0}
		if err := owner.Claim(context.Background(), cfg.OwnerEpoch); err != nil {
			t.Fatal(err)
		}
		cfg.Owner = owner
	}
	cfg.DB = db
	cfg.Spec = compiled
	cfg.TenantID = "default"
	if cfg.Clock == nil {
		cfg.Clock = sources.Physical()
	}
	cfg.IDGenerator = sources.Deterministic()
	if cfg.Executor == nil {
		cfg.Executor = fixture.New()
	}
	cfg.Effector = device.NewSimulatedEffector()
	pipeline, err := runtime.NewPipeline(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func assertShadowWithoutEffects(t *testing.T, db *storage.DB) {
	t.Helper()
	var shadows, intents, commands int
	if err := db.QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM shadow_decisions), (SELECT COUNT(*) FROM intents), (SELECT COUNT(*) FROM commands)").Scan(&shadows, &intents, &commands); err != nil {
		t.Fatal(err)
	}
	if shadows != 1 || intents != 0 || commands != 0 {
		t.Fatalf("shadow decisions=%d intents=%d commands=%d", shadows, intents, commands)
	}
}
