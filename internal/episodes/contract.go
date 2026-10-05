package episodes

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Request is the durable input to an episode executor.
type Request = app.Request

// Outcome is the terminal result of one worker attempt.
type Outcome = app.Outcome

// Executor runs a bounded episode against an Episode Request and returns a
// terminal outcome. Implementations must not mutate stream or action state
// directly; all effects are returned as typed Decisions and Intents.
type Executor = app.Executor

// BudgetExceededError reports that an executor stopped an attempt because it
// consumed more than its admitted budget for Metric.
type BudgetExceededError = domain.BudgetExceededError

// BudgetTelemetryMissingError reports that an executor could not prove an
// attempt stayed within budget because the usage telemetry was absent.
type BudgetTelemetryMissingError = domain.BudgetTelemetryMissingError

// FakeExecutor is the deterministic executor for tests and replay wiring.
type FakeExecutor = app.FakeExecutor

// NewFakeExecutor creates the deterministic fake executor.
func NewFakeExecutor() *FakeExecutor { return app.NewFakeExecutor() }

// CompileIntentCatalog compiles the spec's intents into the canonical catalog
// document and its digest.
func CompileIntentCatalog(intents []spec.Intent) ([]map[string]any, string, error) {
	return domain.CompileIntentCatalog(intents)
}

// Assembler is the public facade over deterministic episode assembly.
type Assembler struct{ application *app.Assembler }

// NewAssembler creates an assembler for the given spec.
func NewAssembler(compiled *spec.CompiledSpec, idGen ids.Generator) *Assembler {
	return &Assembler{application: app.NewAssembler(compiled, idGen)}
}

// WithCostControl enables durable aggregate cost reservation at admission.
func (a *Assembler) WithCostControl(controller *costcontrol.Controller) *Assembler {
	a.application.WithCostControl(controller)
	return a
}

// Assemble builds a Request from a pending scheduler item inside the caller's
// transaction.
func (a *Assembler) Assemble(ctx context.Context, tx *sql.Tx, schedulerItemID, tenantID string) (*Request, error) {
	return a.application.Assemble(ctx, store.Join(tx), schedulerItemID, tenantID)
}

// Persist saves the episode request and marks the scheduler item admitted,
// inside the caller's transaction.
func (a *Assembler) Persist(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	return a.application.Persist(ctx, store.Join(tx), req, now)
}

// Rebind rebuilds an admitted episode's request for the live situation
// version, inside the caller's transaction.
func (a *Assembler) Rebind(ctx context.Context, tx *sql.Tx, req *Request, liveVersion int) (*Request, error) {
	return a.application.Rebind(ctx, store.Join(tx), req, liveVersion)
}

// Runner is the public facade over deterministic episode execution.
type Runner struct{ application *app.Runner }

// NewRunner creates a runner for the given executor and clock.
func NewRunner(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator) *Runner {
	return &Runner{application: app.NewRunner(store.New(db), executor, clk, idGen)}
}

// NewRunnerWithEpoch creates a runner that fences every attempt to ownerEpoch.
func NewRunnerWithEpoch(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator, ownerEpoch string) *Runner {
	return &Runner{application: app.NewRunnerWithEpoch(store.New(db), executor, clk, idGen, ownerEpoch)}
}

// WithEpochControl enables the kill gate: every dispatch validates the
// episode's recorded policy epoch against the control table.
func (r *Runner) WithEpochControl(control *runtimecontrol.EpochControl) *Runner {
	r.application.WithEpochRefusal(epochRefusal(control))
	return r
}

// WithCostControl enables settlement of durable episode cost reservations.
func (r *Runner) WithCostControl(controller *costcontrol.Controller) *Runner {
	r.application.WithCostControl(controller)
	return r
}

// WithShadowStore enables shadow scoring: shadow decisions are scored and
// persisted to shadow_decisions (never to intents/commands).
func (r *Runner) WithShadowStore(store *qualification.ShadowStore) *Runner {
	r.application.WithShadowStore(store)
	return r
}

// WithTelemetry enables the freshness and latency surface.
func (r *Runner) WithTelemetry(runtime *telemetry.Runtime) *Runner {
	r.application.WithTelemetry(runtime)
	return r
}

// WithAssembler enables the re-bind path for stale admitted episodes.
func (r *Runner) WithAssembler(assembler *Assembler) *Runner {
	r.application.WithAssembler(assembler.application)
	return r
}

// RunOnce finds one admitted episode, fences a worker attempt, executes it,
// and persists the attempt terminal state and proposed Decision. It returns
// true if an episode was processed.
func (r *Runner) RunOnce(ctx context.Context, tenantID string) (bool, error) {
	return r.application.RunOnce(ctx, tenantID)
}
