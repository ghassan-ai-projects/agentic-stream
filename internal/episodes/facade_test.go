package episodes_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// The facade is delegation only; this suite pins the public surface — every
// constructor, option and operation reachable without naming an internal
// package.
func TestFacadeConstructorsAndOptions(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "facade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}

	assembler := episodes.NewAssembler(compiled, ids.Deterministic()).WithCostControl(&costcontrol.Controller{})
	if assembler == nil {
		t.Fatal("assembler option chain broke")
	}
	catalog, digest, err := episodes.CompileIntentCatalog(compiled.Actions.Intents)
	if err != nil || len(catalog) == 0 || digest == "" {
		t.Fatalf("catalog = %d digest = %q err = %v", len(catalog), digest, err)
	}

	runner := episodes.NewRunnerWithEpoch(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic(), "epoch").
		WithEpochControl(&control.EpochControl{DB: db}).
		WithCostControl(&costcontrol.Controller{}).
		WithShadowStore(&qualification.ShadowStore{}).
		WithTelemetry(&telemetry.Runtime{}).
		WithAssembler(assembler)
	if runner == nil {
		t.Fatal("runner option chain broke")
	}
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil || processed {
		t.Fatalf("empty run = (%v, %v)", processed, err)
	}
	budget := &episodes.BudgetExceededError{Metric: "wall_time"}
	var missing episodes.BudgetTelemetryMissingError
	if budget.Error() == "" || missing.Error() == "" {
		t.Fatal("budget error texts are empty")
	}
}
