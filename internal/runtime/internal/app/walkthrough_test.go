package app_test

import (
	"os"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

// TestWalkthrough runs the whole pipeline once for a human to inspect: events to
// situation versions, trigger evaluations, an episode, a decision, an intent, a
// policy verdict, a command, an outcome and a verification. It is skipped unless
// AGENTIC_STREAM_WALKTHROUGH names the output database. See
// docs/walkthrough-end-to-end-2026-10-06/README.md.
func TestWalkthrough(t *testing.T) {
	t.Parallel()
	out := os.Getenv("AGENTIC_STREAM_WALKTHROUGH")
	if out == "" {
		t.Skip("set AGENTIC_STREAM_WALKTHROUGH=<db path> to run the walkthrough")
	}
	ctx := t.Context()
	_ = os.Remove(out) //nolint:gosec // The output path is chosen by the person running the walkthrough.
	db, err := storagetest.Open(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled, err := spec.CompileFile(ctx, "testdata/walkthrough.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "default", Clock: sources.Physical(), IDGenerator: sources.Deterministic(), Executor: fixture.New(), Effector: device.NewSimulatedEffector()})
	if err != nil {
		t.Fatal(err)
	}
	report, err := pipeline.RunJSONL(ctx, os.Getenv("AGENTIC_STREAM_WALKTHROUGH_TRACE"))
	t.Logf("report=%+v err=%v", report, err)
}
