package admission_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/admission"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const ownerEpoch = "epoch-admission"

func TestAdmitPendingStampsTheOwnerEpoch(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "native"})
	admitted, err := admitter.AdmitPending(t.Context())
	if err != nil || admitted != 1 {
		t.Fatalf("AdmitPending = %d, %v; want 1 admission", admitted, err)
	}
	if epoch := episodeEpoch(t, db); epoch != ownerEpoch {
		t.Fatalf("policy_epoch = %q, want %q", epoch, ownerEpoch)
	}
	if status := itemStatus(t, db); status != "admitted" {
		t.Fatalf("scheduler item status = %q, want admitted", status)
	}
}

func TestAdmitPendingQuarantinesFixtureOnProductionRoute(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "fixture"})
	admitted, err := admitter.AdmitPending(t.Context())
	if err != nil || admitted != 0 {
		t.Fatalf("AdmitPending = %d, %v; want a quarantined item and no error", admitted, err)
	}
	if status := itemStatus(t, db); status != "coalesced" {
		t.Fatalf("scheduler item status = %q, want coalesced", status)
	}
}

func TestAdmitPendingAdmitsFixtureInDemoMode(t *testing.T) {
	t.Parallel()
	_, admitter := pendingItem(t, scenario{executor: "fixture", demo: true})
	if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != 1 {
		t.Fatalf("AdmitPending = %d, %v; want 1 admission in demo mode", admitted, err)
	}
}

func TestAdmitPendingStopsWhileTheEpochDrains(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "native", drain: true})
	if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != 0 {
		t.Fatalf("AdmitPending = %d, %v; want no admission while draining", admitted, err)
	}
	if status := itemStatus(t, db); status != "pending" {
		t.Fatalf("scheduler item status = %q, want pending", status)
	}
}

func TestAdmitPendingSkipsCostRejectedItems(t *testing.T) {
	t.Parallel()
	db, admitter := pendingItem(t, scenario{executor: "native", costKill: true})
	if admitted, err := admitter.AdmitPending(t.Context()); err != nil || admitted != 0 {
		t.Fatalf("AdmitPending = %d, %v; want a cost-rejected skip", admitted, err)
	}
	if status := itemStatus(t, db); status != "coalesced" {
		t.Fatalf("scheduler item status = %q, want coalesced", status)
	}
}

// scenario is the admission condition a test arranges.
type scenario struct {
	executor              string
	demo, drain, costKill bool
}

// pendingItem ingests one triggering event and runs the stream engine, leaving
// exactly one pending scheduler item for an admitter in the given scenario.
func pendingItem(t *testing.T, given scenario) (*storage.DB, *admission.Admitter) {
	t.Helper()
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "admission.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled := testSpec(given.executor)
	runStream(t, db, compiled)
	return db, admission.New(composeConfig(t, db, compiled, given))
}

func runStream(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec) {
	t.Helper()
	log := eventlog.NewEventLogWithClock(db, clock.Physical())
	stream, err := engine.NewEngine(t.Context(), db, log, clock.Physical(), compiled, "default")
	if err != nil {
		t.Fatal(err)
	}
	path := traceFile(t)
	if _, err := ingress.NewJSONLReplay(db, log, "default", path, "test:"+path).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.RunGlobal(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func composeConfig(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, given scenario) admission.Config {
	t.Helper()
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "admission-instance"}
	if err := owner.Claim(t.Context(), ownerEpoch); err != nil {
		t.Fatal(err)
	}
	control := &runtimecontrol.EpochControl{DB: db}
	if given.drain {
		if err := control.Drain(t.Context(), ownerEpoch); err != nil {
			t.Fatal(err)
		}
	}
	if given.costKill {
		setCostKillSwitch(t, db)
	}
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: ids.Deterministic(), CostControl: &costcontrol.Controller{}})
	if err != nil {
		t.Fatal(err)
	}
	return admission.Config{
		DB: db, Episodes: assembler, Clock: clock.Physical(), TenantID: "default",
		Owner: owner, OwnerEpoch: ownerEpoch, EpochControl: control, DemoMode: given.demo,
	}
}

func setCostKillSwitch(t *testing.T, db *storage.DB) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return costcontrol.SetLimit(t.Context(), tx, "global", "", 0, true, time.Now().UTC().Format(time.RFC3339Nano))
	}); err != nil {
		t.Fatal(err)
	}
}

func traceFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "event.jsonl")
	trace := `{"id":"evt-1","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":15}}` + "\n"
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testSpec(executorName string) *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs:    []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:      spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
		Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}},
			Executor: spec.Executor{Name: executorName, DispatchPolicy: "shadow", ModelPolicy: "test", PromptVersion: "v1",
				DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}}},
		Actions: spec.Actions{Intents: []spec.Intent{{Type: "create_maintenance_ticket", Risk: "R1", Policy: "automatic",
			ParameterSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}}}}}},
	}
}

func episodeEpoch(t *testing.T, db *storage.DB) string {
	t.Helper()
	var epoch string
	if err := db.QueryRowContext(t.Context(), "SELECT policy_epoch FROM episodes").Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	return epoch
}

func itemStatus(t *testing.T, db *storage.DB) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM scheduler_items").Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}
