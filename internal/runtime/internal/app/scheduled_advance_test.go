package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestAdvanceEveryStartsDueCognitionWithoutNewEvidence(t *testing.T) {
	t.Parallel()
	clock := sources.NewVirtual(time.Date(2026, 8, 12, 0, 0, 2, 0, time.UTC))
	pipeline, db := openPipeline(t, thingSpec("native", "active", withDebounce("5s")), runtime.PipelineConfig{OwnerEpoch: "epoch-advance", Clock: clock})

	ingested, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t))
	if err != nil || ingested.EpisodesAdmitted != 0 {
		t.Fatalf("a debounced trigger must wait while its debounce runs: report=%+v err=%v", ingested, err)
	}
	clock.Advance(6 * time.Second)
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- pipeline.AdvanceEvery(ctx, time.Millisecond) }()

	waitForRows(t, db, "commands")
	stop()
	if err := <-done; err != nil {
		t.Fatalf("advance loop: %v", err)
	}
}

func TestScheduledLoopsRefuseANonPositiveInterval(t *testing.T) {
	t.Parallel()
	loops := map[string]func(*runtime.Pipeline, context.Context, time.Duration) error{
		"advance":  (*runtime.Pipeline).AdvanceEvery,
		"episodes": (*runtime.Pipeline).RunEpisodesEvery,
	}
	for name, loop := range loops {
		for _, interval := range []time.Duration{0, -time.Second} {
			t.Run(name+" "+interval.String(), func(t *testing.T) {
				t.Parallel()
				pipeline, _ := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
				if err := loop(pipeline, t.Context(), interval); err == nil {
					t.Fatalf("interval %s was accepted", interval)
				}
			})
		}
	}
}

func TestScheduledLoopsStopWithTheFailureThatBrokeThem(t *testing.T) {
	t.Parallel()
	loops := map[string]func(*runtime.Pipeline, context.Context, time.Duration) error{
		"advance":  (*runtime.Pipeline).AdvanceEvery,
		"episodes": (*runtime.Pipeline).RunEpisodesEvery,
	}
	for name, loop := range loops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := loop(pipeline, t.Context(), time.Millisecond); err == nil {
				t.Fatal("a loop over a closed database ended without an error")
			}
		})
	}
}
