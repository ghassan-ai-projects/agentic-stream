package app_test

import (
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

// Epoch controls prevent new admission and later execution after drain/kill.

// Drain refuses NEW admission but the recorded epoch stays untouched.
func TestEpochDrainRefusesNewAdmission(t *testing.T) {
	db := openModeDB(t, "p8-drain.db")
	control := &runtimecontrol.EpochControl{DB: db}
	epoch := "epoch-p8-drain"
	compiled := modeCompiledSpec("native", "active")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: epoch, EpochControl: control,
	})
	if _, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15)); err != nil {
		t.Fatalf("run under a live epoch: %v", err)
	}
	if err := control.Drain(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	if err := control.AssertAdmission(t.Context(), epoch); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("expected ErrEpochDraining after drain, got %v", err)
	}
	second, err := pipeline.RunJSONL(t.Context(), modeTraceForEntity(t, 15, "evt-after-drain", "ent-after-drain"))
	if err != nil {
		t.Fatal(err)
	}
	if second.EventsIngested != 1 || second.EventsProcessed < 1 || second.EpisodesAdmitted != 0 || second.EpisodesExecuted != 0 || second.CommandsDispatched != 0 {
		t.Fatalf("drained epoch admitted new entity: %+v", second)
	}

}

// Kill refuses every later decision under the killed epoch.
func TestEpochKillRefusesLaterDecisions(t *testing.T) {
	db := openModeDB(t, "p8-kill.db")
	control := &runtimecontrol.EpochControl{DB: db}
	epoch := "epoch-p8-kill"
	if err := control.Kill(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	if err := control.AssertDecision(t.Context(), epoch); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("expected ErrEpochKilled after kill, got %v", err)
	}
	// A different (uncontrolled) epoch is untouched — a kill is scoped to its
	// epoch.
	if err := control.AssertDecision(t.Context(), "epoch-other"); err != nil {
		t.Fatalf("uncontrolled epoch must not be refused: %v", err)
	}
}

// Fresh evidence still advances the stream after kill, but cannot start new
// episodes or effects under the killed epoch.
func TestEpochKillPreventsNewPipelineWork(t *testing.T) {
	db := openModeDB(t, "p8-kill-dispatch.db")
	control := &runtimecontrol.EpochControl{DB: db}
	epoch := "epoch-p8-kill-dispatch"
	compiled := modeCompiledSpec("native", "active")
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{
		OwnerEpoch: epoch, EpochControl: control,
	})
	// Admit + execute one episode under the live epoch.
	first, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
	if err != nil {
		t.Fatalf("run under a live epoch: %v", err)
	}
	if first.EpisodesAdmitted != 1 {
		t.Fatalf("expected one admission, got %+v", first)
	}
	// Kill the epoch, then ingest a fresh event for a different entity.
	// Replaying the earlier event could pass through duplicate suppression.
	if err := control.Kill(t.Context(), epoch); err != nil {
		t.Fatal(err)
	}
	second, err := pipeline.RunJSONL(t.Context(), modeTraceForEntity(t, 15, "evt-after-kill", "ent-after-kill"))
	if err != nil {
		t.Fatalf("a killed epoch must skip admission, not fail the batch: %v", err)
	}
	if second.EventsIngested != 1 || second.EventsProcessed < 1 || second.EpisodesAdmitted != 0 || second.EpisodesExecuted != 0 || second.CommandsDispatched != 0 {
		t.Fatalf("no episode may run under a killed epoch, got %+v", second)
	}
	if err := control.AssertDecision(t.Context(), epoch); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("expected the recorded epoch to be killed, got %v", err)
	}
}
