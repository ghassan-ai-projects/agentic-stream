package composition_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestPipelineRunsNormalizedBatchThroughAllPlanes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	compiled, err := spec.CompileFile(ctx, "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", Clock: sources.Physical(), IDGenerator: sources.Deterministic(),
		Executor: fixture.New(), Effector: device.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := pipeline.RunJSONL(ctx, "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if report.EventsIngested != 1 || report.EventsProcessed != 1 {
		t.Fatalf("unexpected pipeline report: %+v", report)
	}
}

func TestPipelineSurvivesWatchExpressionEvaluationError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	compiled, err := spec.CompileFile(ctx, "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}

	watch := newWatch(t, db)
	command := actionport.Command{
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
		DB: db, Spec: compiled, TenantID: "default", Clock: sources.Physical(), IDGenerator: sources.Deterministic(),
		Executor: fixture.New(), Effector: device.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunJSONL(ctx, "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"); err != nil {
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

func TestPipelineRecordsTheConfiguredCostCeilingsBeforeAnyWork(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled, err := spec.CompileFile(t.Context(), "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	global, tenant, kill := uint64(500), uint64(200), true
	if _, err := runtime.NewPipeline(t.Context(), runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "tenant-a", GlobalCostCeiling: &global, TenantCostCeiling: &tenant, CostKillSwitch: &kill}); err != nil {
		t.Fatal(err)
	}
	limits := map[string]int{}
	rows, err := db.QueryContext(t.Context(), "SELECT scope_key, max_micro FROM cost_limits")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var scope string
		var max int
		if err := rows.Scan(&scope, &max); err != nil {
			t.Fatal(err)
		}
		limits[scope] = max
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(limits) != 2 || limits["global"] != 500 || limits["tenant:tenant-a"] != 200 {
		t.Fatalf("cost limits = %v, want global 500 and tenant:tenant-a 200", limits)
	}
}
