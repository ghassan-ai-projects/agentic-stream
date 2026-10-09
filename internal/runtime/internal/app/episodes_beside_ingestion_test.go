package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

type reasoningExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (r *reasoningExecutor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return fixture.New().Execute(ctx, req)
}

func TestEpisodesRunBesideIngestion(t *testing.T) {
	t.Parallel()
	worker := &reasoningExecutor{started: make(chan struct{}, 1), release: make(chan struct{})}
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-beside", Executor: worker})
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	loopDone := make(chan error, 1)
	go func() { loopDone <- pipeline.RunEpisodesEvery(ctx, 10*time.Millisecond) }()

	admitted := ingestWhileReasoning(t, pipeline, writeTrace(t, levelEvent("evt-1", "ent-1", 15)))
	waitForSignal(t, worker.started, "the episode loop never started the attempt")
	ingested := ingestWhileReasoning(t, pipeline, writeTrace(t, levelEvent("evt-2", "ent-2", 15)))
	if admitted.EpisodesExecuted != 0 || ingested.EventsIngested != 1 {
		t.Fatalf("batches must leave episodes to the loop and keep ingesting: first=%+v second=%+v", admitted, ingested)
	}
	if commands := countRows(t, db, "commands"); commands != 0 {
		t.Fatalf("%d commands before the worker decided", commands)
	}
	close(worker.release)
	advanceDone := make(chan error, 1)
	go func() { advanceDone <- pipeline.AdvanceEvery(ctx, 10*time.Millisecond) }()
	waitForRows(t, db, "commands")
	stop()
	if err := <-loopDone; err != nil {
		t.Fatalf("episode loop: %v", err)
	}
	if err := <-advanceDone; err != nil {
		t.Fatalf("advance loop: %v", err)
	}
}

func ingestWhileReasoning(t *testing.T, pipeline *runtime.Pipeline, trace string) runtime.PipelineReport {
	t.Helper()
	done := make(chan runtime.PipelineReport, 1)
	go func() {
		report, err := pipeline.RunJSONL(t.Context(), trace)
		if err != nil {
			t.Errorf("ingest: %v", err)
		}
		done <- report
	}()
	select {
	case report := <-done:
		return report
	case <-time.After(10 * time.Second):
		t.Fatal("ingestion waited for a reasoning worker")
		return runtime.PipelineReport{}
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal(failure)
	}
}
