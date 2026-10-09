package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

// Dispatch modes bind executor identity to active/shadow and demo admission.

// The full mode×policy matrix: a production route rejects `fixture` for every
// dispatch policy, while a demo mode admits it. The executor name is read from
// the spec, so each cell is a spec variant. The rejected item is quarantined
// (coalesced), never admitted — the queue keeps draining.
func TestDispatchModeFixtureRejectedOnProductionRoutes(t *testing.T) {
	t.Parallel()
	for _, dispatchPolicy := range []string{"active", "shadow"} {
		dispatchPolicy := dispatchPolicy
		t.Run("fixture_"+dispatchPolicy, func(t *testing.T) {
			t.Parallel()
			db := openModeDB(t, "p8-fixture.db")
			compiled := modeCompiledSpec("fixture", dispatchPolicy)
			pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{
				OwnerEpoch: "epoch-p8-fixture",
			})
			report, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
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
func TestDispatchModeFixtureAdmittedInDemoMode(t *testing.T) {
	db := openModeDB(t, "p8-demo.db")
	compiled := modeCompiledSpec("fixture", "shadow")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: "epoch-p8-demo", DemoMode: true,
	})
	report, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
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
func TestDispatchModeNativeActiveRecordsPolicy(t *testing.T) {
	db := openModeDB(t, "p8-native-active.db")
	compiled := modeCompiledSpec("native", "active")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-native"})
	report, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
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
func TestDispatchModeNativeShadowIsScoredNotDispatched(t *testing.T) {
	assertShadowIsScoredNotDispatched(t, "native", "p8-native-shadow.db", "epoch-p8-ns")
}

// tamoz×active: the configured executor name is admitted under active policy.
// This fixture exercises admission and dispatch policy, not a remote worker.
func TestDispatchModeTamozActiveDispatches(t *testing.T) {
	db := openModeDB(t, "p8-tamoz-active.db")
	compiled := modeCompiledSpec("tamoz", "active")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-p8-ta"})
	report, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("tamoz+active must dispatch, got %+v", report)
	}
}

// tamoz×shadow: admitted and scored, never dispatched to actions.
func TestDispatchModeTamozShadowIsScoredNotDispatched(t *testing.T) {
	assertShadowIsScoredNotDispatched(t, "tamoz", "p8-tamoz-shadow.db", "epoch-p8-ts")
}

func assertShadowIsScoredNotDispatched(t *testing.T, executor, dbName, epoch string) {
	t.Helper()
	db := openModeDB(t, dbName)
	compiled := modeCompiledSpec(executor, "shadow")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: epoch})
	report, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EpisodesAdmitted != 1 || report.CommandsDispatched != 0 {
		t.Fatalf("%s+shadow must never dispatch, got %+v", executor, report)
	}
	assertShadowWithoutEffects(t, db)
}
