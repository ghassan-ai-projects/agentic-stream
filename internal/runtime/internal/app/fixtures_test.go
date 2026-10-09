package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const zeroDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

type specChange func(*spec.CompiledSpec)

func thingSpec(executorName, dispatchPolicy string, changes ...specChange) *spec.CompiledSpec {
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: zeroDigest,
		Inputs:    []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}},
			Executor: spec.Executor{Name: executorName, DispatchPolicy: dispatchPolicy, ModelPolicy: "test", PromptVersion: "v1",
				DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}, RiskCeiling: "R1"}},
		Actions: spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema(), Policy: "automatic"}}},
	}
	for _, change := range changes {
		change(compiled)
	}
	return compiled
}

func withCostMicrounits(micros int) specChange {
	return func(s *spec.CompiledSpec) { s.Cognition.Executor.Budget.CostMicrounits = micros }
}

func withLateCorrection(s *spec.CompiledSpec) {
	s.Time = spec.TimePolicy{MaxOutOfOrderness: "0s", AllowedLateness: "5m", LatePolicy: "correct_and_reconsider"}
	s.Windows[0].Size = "5m"
	s.Cognition.Triggers[0].MaterialDelta = "delta.phase_changed"
}

func withSimulatorInput(s *spec.CompiledSpec) {
	s.Inputs[0].EventType = "thing.temperature.observed"
	s.Operators[0].Field = "data.celsius"
}

func withDebounce(debounce string) specChange {
	return func(s *spec.CompiledSpec) { s.Cognition.Triggers[0].Debounce = debounce }
}

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}

type traceEvent struct {
	id, entity         string
	level              int
	eventTime, ingests string
}

func levelEvent(id, entity string, level int) traceEvent {
	return traceEvent{id: id, entity: entity, level: level, eventTime: "2026-08-12T00:00:00Z", ingests: "2026-08-12T00:00:01Z"}
}

func (e traceEvent) json() string {
	return fmt.Sprintf(`{"id":%q,"type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":%q,"entity":{"type":"thing","id":%q},"event_time":%q,"ingested_at":%q,"classification":"internal","data":{"level":%d}}`,
		e.id, e.entity, e.entity, e.eventTime, e.ingests, e.level)
}

func writeTrace(t *testing.T, events ...traceEvent) string {
	t.Helper()
	lines := make([]string, len(events))
	for i, event := range events {
		lines[i] = event.json()
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSimulatorTrace(t *testing.T, value int) string {
	t.Helper()
	lines := []string{
		`{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}`,
		fmt.Sprintf(`{"record_type":"event","event":{"id":"sim-1","entity_type":"thing","entity_id":"thing-1","type":"temperature","event_time":"2026-08-12T00:00:00Z","arrival_time":"2026-08-12T00:00:01Z","value":%d}}`, value),
		`{"record_type":"trace_end","until":"2026-08-12T00:01:00Z"}`,
	}
	path := filepath.Join(t.TempDir(), "simulator.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func highLevelTrace(t *testing.T) string {
	t.Helper()
	return writeTrace(t, levelEvent("evt-1", "ent-1", 15))
}

func newPipeline(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, cfg runtime.PipelineConfig) *runtime.Pipeline {
	t.Helper()
	if cfg.OwnerEpoch != "" && cfg.Owner == nil {
		cfg.Owner = claimOwnership(t, db, cfg.OwnerEpoch)
	}
	cfg.DB, cfg.Spec, cfg.TenantID = db, compiled, "default"
	cfg.Clock = sources.OrPhysical(cfg.Clock)
	if cfg.IDGenerator == nil {
		cfg.IDGenerator = sources.Deterministic()
	}
	if cfg.Executor == nil {
		cfg.Executor = fixture.New()
	}
	if cfg.Effector == nil {
		cfg.Effector = device.NewSimulatedEffector()
	}
	pipeline, err := runtime.NewPipeline(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeline.Close() })
	return pipeline
}

func claimOwnership(t *testing.T, db *storage.DB, epoch string) *runtimecontrol.RuntimeOwner {
	t.Helper()
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: epoch + "-instance"}
	if err := owner.Claim(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	return owner
}

func openDatabase(t *testing.T) *storage.DB {
	t.Helper()
	return storagetest.OpenTemp(t)
}

func openPipeline(t *testing.T, compiled *spec.CompiledSpec, cfg runtime.PipelineConfig) (*runtime.Pipeline, *storage.DB) {
	t.Helper()
	db := openDatabase(t)
	return newPipeline(t, db, compiled, cfg), db
}

func scalar[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	return scalar[int](t, db, "SELECT COUNT(*) FROM "+table) //nolint:gosec // fixed table names
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", what)
		case <-ticker.C:
		}
	}
}

func waitForRows(t *testing.T, db *storage.DB, table string) {
	t.Helper()
	waitUntil(t, "a "+table+" row", func() bool { return countRows(t, db, table) > 0 })
}

func assertShadowWithoutEffects(t *testing.T, db *storage.DB) {
	t.Helper()
	shadows, intents, commands := countRows(t, db, "shadow_decisions"), countRows(t, db, "intents"), countRows(t, db, "commands")
	if shadows != 1 || intents != 0 || commands != 0 {
		t.Fatalf("shadow decisions=%d intents=%d commands=%d; want 1, 0, 0", shadows, intents, commands)
	}
}
