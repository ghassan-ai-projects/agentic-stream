package runtime_test

import (
	"context"
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

func TestPipelineRunsNormalizedBatchThroughAllPlanes(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "pipeline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled, err := spec.CompileFile(ctx, "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(),
		Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := pipeline.RunJSONL(ctx, "../../examples/predictive-maintenance/testdata/trace-opening.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if report.EventsIngested != 1 || report.EventsProcessed != 1 {
		t.Fatalf("unexpected pipeline report: %+v", report)
	}
}

func TestPipelineSurvivesWatchExpressionEvaluationError(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "pipeline-watch-evaluation-error.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled, err := spec.CompileFile(ctx, "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}

	watch := actions.NewWatchEffector(db)
	command := actions.Command{
		CommandID: "cmd-pipeline-watch-evaluation-error", TenantID: "default", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression": "features.do_mean_15m > 0", "target": "motor-17", "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "sit-1", "situation_version": 1, "max_fires": 1,
		},
	}
	if _, err := watch.Dispatch(ctx, command); err != nil {
		t.Fatal(err)
	}

	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", Clock: clock.Physical(), IDGenerator: ids.Deterministic(),
		Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunJSONL(ctx, "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"); err != nil {
		t.Fatalf("pipeline stopped on watch expression evaluation error: %v", err)
	}

	var status string
	var remaining int
	if err := db.QueryRowContext(ctx, "SELECT status, remaining_fires FROM watch_conditions WHERE watch_id = ?", command.CommandID).Scan(&status, &remaining); err != nil {
		t.Fatal(err)
	}
	if status != "active" || remaining != 1 {
		t.Fatalf("watch after pipeline batch = status %q, remaining fires %d; want active, 1", status, remaining)
	}
}
