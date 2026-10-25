package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestAFedEventFlowsThroughEveryGovernedStageUntilShutdown(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	runDone := make(chan error, 1)
	go func() {
		runDone <- pipeline.RunFeed(ctx, func(ctx context.Context, emit func(context.Context, contractsv1.Envelope) error) error {
			if err := emit(ctx, liveEnvelope("evt-fed-situation", "thing-1", 15)); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	waitUntil(t, "the fed event to be dispatched", func() bool {
		return scalar[int](t, db, "SELECT COUNT(*) FROM commands WHERE status = 'succeeded'") == 1
	})
	awaitLiveBatchEnd(t, pipeline)
	shutdown()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunFeed = %v after shutdown, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the fed pipeline did not shut down")
	}
}

func TestRunFeedReportsAFailingFeed(t *testing.T) {
	t.Parallel()
	pipeline, _ := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	failure := errors.New("node gone")
	err := pipeline.RunFeed(t.Context(), func(context.Context, func(context.Context, contractsv1.Envelope) error) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("RunFeed = %v, want the feed's error", err)
	}
}
