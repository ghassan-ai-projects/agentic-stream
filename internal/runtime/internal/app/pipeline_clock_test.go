package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// A debounced trigger queues its episode for later. On a quiet live source no
// new evidence arrives to advance the pipeline, so the scheduled advance must
// start it once it is due, and must not start it early.
func TestAdvanceStartsDueCognitionWithoutNewEvidence(t *testing.T) {
	t.Parallel()
	db := openModeDB(t, "advance.db")
	compiled := modeCompiledSpec("native", "active")
	compiled.Cognition.Triggers[0].Debounce = "5s"
	clock := sources.NewVirtual(time.Date(2026, 8, 12, 0, 0, 2, 0, time.UTC))
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-advance", Clock: clock})

	ingested, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
	if err != nil || ingested.EpisodesAdmitted != 0 {
		t.Fatalf("debounced trigger must wait: report=%+v err=%v", ingested, err)
	}
	early, err := pipeline.Advance(t.Context())
	if err != nil || early.EpisodesAdmitted != 0 {
		t.Fatalf("advance before the debounce must not admit: report=%+v err=%v", early, err)
	}
	clock.Advance(6 * time.Second)
	due, err := pipeline.Advance(t.Context())
	if err != nil || due.EpisodesAdmitted != 1 || due.CommandsDispatched != 1 {
		t.Fatalf("advance after the debounce must admit and dispatch: report=%+v err=%v", due, err)
	}
}

func TestAdvanceEveryRejectsANonPositiveInterval(t *testing.T) {
	t.Parallel()
	db := openModeDB(t, "advance-interval.db")
	pipeline := newModePipeline(t, db, modeCompiledSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-interval"})
	if err := pipeline.AdvanceEvery(t.Context(), 0); err == nil {
		t.Fatal("a zero interval must be refused")
	}
}
