package app_test

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// withEpochRunner builds a runner with the kill gate wired over db.
func withEpochRunner(db *storage.DB, executor app.Executor, clk clock.Clock, idGen ids.Generator) *app.Runner {
	return withEpochControl(app.NewRunner(store.New(db), executor, clk, idGen), &runtimecontrol.EpochControl{DB: db})
}

// withEpochControl wires the production refusal mapping for app tests; the
// public facade injects the same projection at composition time.
func withEpochControl(r *app.Runner, control *runtimecontrol.EpochControl) *app.Runner {
	return r.WithDecisionEpoch(control.AssertDecisionTx)
}
