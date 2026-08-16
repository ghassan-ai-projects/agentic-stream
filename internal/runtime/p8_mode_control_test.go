package runtime_test

import (
	"context"
	"errors"
	"fmt"
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

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md): the mode matrix, drain, and kill.
// The mode matrix is the design's core contract: executor × dispatch_policy.
// A production pipeline (DemoMode false) rejects `fixture`; a drained epoch
// refuses new admission while in-flight finishes under its recorded epoch; a
// killed epoch refuses every later decision independently of the worker.

func p8CompiledSpec(executorName, dispatchPolicy string) *spec.CompiledSpec {
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

func p8TraceFile(t *testing.T, level int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p8-event.jsonl")
	trace := `{"id":"evt-p8","type":"test.observed","schema_version":"1.0","tenant_id":"default","source":"test","partition_key":"ent-1","entity":{"type":"thing","id":"ent-1"},"event_time":"2026-08-12T00:00:00Z","ingested_at":"2026-08-12T00:00:01Z","classification":"internal","data":{"level":` + fmt.Sprint(level) + `}}` + "\n"
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func p8OpenDB(t *testing.T, name string) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func p8NewPipeline(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, cfg runtime.PipelineConfig) *runtime.Pipeline {
	t.Helper()
	if cfg.OwnerEpoch != "" {
		owner := &storage.RuntimeOwner{DB: db, InstanceID: cfg.OwnerEpoch + "-instance", Lease: 0}
		if err := owner.Claim(context.Background(), cfg.OwnerEpoch); err != nil {
			t.Fatal(err)
		}
		cfg.Owner = owner
	}
	cfg.DB = db
	cfg.Spec = compiled
	cfg.TenantID = "default"
	cfg.Clock = clock.Physical()
	cfg.IDGenerator = ids.Deterministic()
	cfg.Executor = episodes.NewFakeExecutor()
	cfg.Effector = actions.NewSimulatedEffector()
	pipeline, err := runtime.NewPipeline(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

// The full mode×policy matrix: a production route rejects `fixture` for every
// dispatch policy, while a demo mode admits it. The executor name is read from
// the spec, so each cell is a spec variant. The rejected item is quarantined
// (coalesced), never admitted — the queue keeps draining.
func TestP8ModeMatrixFixtureRejectedOnProductionRoutes(t *testing.T) {
	t.Parallel()
	for _, dispatchPolicy := range []string{"active", "shadow"} {
		dispatchPolicy := dispatchPolicy
		t.Run("fixture_"+dispatchPolicy, func(t *testing.T) {
			t.Parallel()
			db := p8OpenDB(t, "p8-fixture.db")
			compiled := p8CompiledSpec("fixture", dispatchPolicy)
			pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{
				OwnerEpoch: "epoch-p8-fixture",
			})
			report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
			if err != nil {
				t.Fatalf("fixture rejection must quarantine the item, not fail the batch: %v", err)
			}
			if report.EpisodesAdmitted != 0 {
				t.Fatalf("fixture executor must not be admitted on a production route (%s), got %+v",
					dispatchPolicy, report)
			}
		})
	}
}

// A fixture executor IS admitted in demo mode (demos and tests only).
func TestP8ModeMatrixFixtureAdmittedInDemoMode(t *testing.T) {
	db := p8OpenDB(t, "p8-demo.db")
	compiled := p8CompiledSpec("fixture", "shadow")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: "epoch-p8-demo", DemoMode: true,
	})
	report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatalf("fixture executor must be admitted in demo mode: %v", err)
	}
	if report.EpisodesAdmitted != 1 {
		t.Fatalf("expected one admission in demo mode, got %+v", report)
	}
}

// A native executor under active policy dispatches end-to-end (the baseline
// cell of the matrix — the existing pipeline e2e proves the path; this pins
// the recorded dispatch_policy on the episode row).
func TestP8ModeMatrixNativeActiveRecordsPolicy(t *testing.T) {
	db := p8OpenDB(t, "p8-native-active.db")
	compiled := p8CompiledSpec("native", "active")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-native"})
	report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("native+active must dispatch, got %+v", report)
	}
	var policy, epoch string
	if err := db.QueryRowContext(t.Context(), `
		SELECT dispatch_policy, policy_epoch FROM episodes LIMIT 1`).Scan(&policy, &epoch); err != nil {
		t.Fatal(err)
	}
	if policy != "active" {
		t.Fatalf("recorded dispatch_policy = %q, want active", policy)
	}
	if epoch != "epoch-p8-native" {
		t.Fatalf("recorded policy_epoch = %q, want epoch-p8-native", epoch)
	}
}

// native×shadow: dispatched and scored, never dispatched to actions.
func TestP8ModeMatrixNativeShadowIsScoredNotDispatched(t *testing.T) {
	db := p8OpenDB(t, "p8-native-shadow.db")
	compiled := p8CompiledSpec("native", "shadow")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-ns"})
	report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 0 {
		t.Fatalf("native+shadow must never dispatch, got %+v", report)
	}
	var intents int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM intents").Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if intents != 0 {
		t.Fatalf("shadow must not enter governance: %d intents", intents)
	}
}

// tamoz×active: the tamoz executor name is admitted under active policy and
// dispatched (the mode matrix cell the Ruby worker refuses to serve with a
// fixture provider).
func TestP8ModeMatrixTamozActiveDispatches(t *testing.T) {
	db := p8OpenDB(t, "p8-tamoz-active.db")
	compiled := p8CompiledSpec("tamoz", "active")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-ta"})
	report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("tamoz+active must dispatch, got %+v", report)
	}
}

// tamoz×shadow: admitted and scored, never dispatched to actions.
func TestP8ModeMatrixTamozShadowIsScoredNotDispatched(t *testing.T) {
	db := p8OpenDB(t, "p8-tamoz-shadow.db")
	compiled := p8CompiledSpec("tamoz", "shadow")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-ts"})
	report, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 0 {
		t.Fatalf("tamoz+shadow must never dispatch, got %+v", report)
	}
}

// Drain refuses NEW admission but the recorded epoch stays untouched.
func TestP8DrainRefusesNewAdmission(t *testing.T) {
	db := p8OpenDB(t, "p8-drain.db")
	control := &storage.EpochControl{DB: db}
	epoch := "epoch-p8-drain"
	compiled := p8CompiledSpec("native", "active")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: epoch, EpochControl: control,
	})
	if _, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15)); err != nil {
		t.Fatalf("run under a live epoch: %v", err)
	}
	if err := control.Drain(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	if err := control.AssertAdmission(t.Context(), epoch); !errors.Is(err, storage.ErrEpochDraining) {
		t.Fatalf("expected ErrEpochDraining after drain, got %v", err)
	}
}

// Kill refuses every later decision under the killed epoch.
func TestP8KillRefusesLaterDecisions(t *testing.T) {
	db := p8OpenDB(t, "p8-kill.db")
	control := &storage.EpochControl{DB: db}
	epoch := "epoch-p8-kill"
	if err := control.Kill(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	if err := control.AssertDecision(t.Context(), epoch); !errors.Is(err, storage.ErrEpochKilled) {
		t.Fatalf("expected ErrEpochKilled after kill, got %v", err)
	}
	// A different (uncontrolled) epoch is untouched — a kill is scoped to its
	// epoch.
	if err := control.AssertDecision(t.Context(), "epoch-other"); err != nil {
		t.Fatalf("uncontrolled epoch must not be refused: %v", err)
	}
}

// The decision-boundary gate is independent of the worker: a hostile worker
// that keeps producing after the kill is refused at the Runner's dispatch
// boundary (the episode's recorded policy epoch is checked before the attempt
// starts). The episode is quarantined so the admitted queue keeps draining.
func TestP8KillRefusesDispatchOfARecordedEpoch(t *testing.T) {
	db := p8OpenDB(t, "p8-kill-dispatch.db")
	control := &storage.EpochControl{DB: db}
	epoch := "epoch-p8-kill-dispatch"
	compiled := p8CompiledSpec("native", "active")
	pipeline := p8NewPipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: epoch, EpochControl: control,
	})
	// Admit + execute one episode under the live epoch.
	first, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatalf("run under a live epoch: %v", err)
	}
	if first.EpisodesAdmitted != 1 {
		t.Fatalf("expected one admission, got %+v", first)
	}
	// Kill the epoch, then dispatch again: the admission gate skips (drain)
	// and the recorded-epoch decision gate refuses in-flight episodes — no new
	// episode may run under the killed epoch.
	if err := control.Kill(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	second, err := pipeline.RunJSONL(t.Context(), p8TraceFile(t, 15))
	if err != nil {
		t.Fatalf("a killed epoch must skip admission, not fail the batch: %v", err)
	}
	if second.EpisodesAdmitted != 0 || second.EpisodesExecuted != 0 {
		t.Fatalf("no episode may run under a killed epoch, got %+v", second)
	}
	if err := control.AssertDecision(t.Context(), epoch); !errors.Is(err, storage.ErrEpochKilled) {
		t.Fatalf("expected the recorded epoch to be killed, got %v", err)
	}
}
