package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

// reasoningExecutor stands for a worker that takes its time: it reports when
// an attempt starts and decides only when released.
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

// While a worker reasons, the live pipeline keeps ingesting and advancing; the
// decision is governed and dispatched once it arrives (ADR-018).
func TestEpisodesRunBesideIngestion(t *testing.T) {
	t.Parallel()
	worker := &reasoningExecutor{started: make(chan struct{}, 1), release: make(chan struct{})}
	db := openModeDB(t, "beside.db")
	pipeline := newModePipeline(t, db, modeCompiledSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-beside", Executor: worker})
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	loopDone := make(chan error, 1)
	go func() { loopDone <- pipeline.RunEpisodesEvery(ctx, 10*time.Millisecond) }()

	admitted := runSource(t, pipeline, modeTraceForEntity(t, 15, "evt-1", "ent-1"))
	waitFor(t, worker.started, "the episode loop never started the attempt")
	ingested := runSource(t, pipeline, modeTraceForEntity(t, 15, "evt-2", "ent-2"))
	if admitted.EpisodesExecuted != 0 || ingested.EventsIngested != 1 {
		t.Fatalf("batches must leave episodes to the loop and keep ingesting: first=%+v second=%+v", admitted, ingested)
	}
	close(worker.release)
	go func() { _ = pipeline.AdvanceEvery(ctx, 10*time.Millisecond) }()
	waitForRows(t, db, "commands")
	stop()
	if err := <-loopDone; err != nil {
		t.Fatalf("episode loop: %v", err)
	}
}

// runSource ingests one trace and must return while an attempt is still
// reasoning.
func runSource(t *testing.T, pipeline *runtime.Pipeline, trace string) runtime.PipelineReport {
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

func waitFor(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal(failure)
	}
}

func TestRunEpisodesEveryRejectsANonPositiveInterval(t *testing.T) {
	t.Parallel()
	pipeline := newModePipeline(t, openModeDB(t, "episodes-interval.db"), modeCompiledSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-episodes-interval"})
	if err := pipeline.RunEpisodesEvery(t.Context(), 0); err == nil {
		t.Fatal("a zero interval must be refused")
	}
}
