package episodes

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Executor runs a bounded episode against an Episode Request and returns a
// terminal outcome. Implementations must not mutate stream or action state
// directly; all effects are returned as typed Decisions and Intents.
type Executor interface {
	// Execute runs the episode to completion or budget exhaustion.
	Execute(ctx context.Context, req *Request) (*Outcome, error)
}

// Outcome is the terminal result of one worker attempt.
type Outcome struct {
	Status         string   `json:"status"`
	AttemptID      string   `json:"attempt_id,omitempty"`
	Fence          int64    `json:"fence,omitempty"`
	DecisionJSON   []byte   `json:"decision_json,omitempty"`
	DecisionSHA256 string   `json:"decision_sha256,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	CostMicrounits uint64   `json:"cost_microunits,omitempty"`
}

// BudgetExceededError reports that an executor stopped an attempt because it
// consumed more than its admitted budget for Metric.
type BudgetExceededError struct{ Metric string }

func (e *BudgetExceededError) Error() string { return "episode budget exceeded: " + e.Metric }

// BudgetTelemetryMissingError reports that an executor could not prove an
// attempt stayed within budget because the usage telemetry was absent.
type BudgetTelemetryMissingError struct{}

func (BudgetTelemetryMissingError) Error() string { return "episode budget telemetry is missing" }

// Runner polls admitted episodes and executes them deterministically.
type Runner struct {
	db           *storage.DB
	executor     Executor
	clk          clock.Clock
	idGen        ids.Generator
	ownerEpoch   string
	cost         *costcontrol.Controller
	epochControl *runtimecontrol.EpochControl
	shadowStore  *qualification.ShadowStore
	telemetry    *telemetry.Runtime
	assembler    *Assembler
}

const maxEpisodeAttempts = 3

// maxStaleRebinds bounds how many times an admitted episode may be re-bound to
// a newer live situation version before it is abandoned as stale. Dense
// entities churn versions faster than the dispatch poll; the durable
// stale_rebind_count column tracks the budget across batches and restarts.
const maxStaleRebinds = 3

// WithCostControl enables settlement of durable episode cost reservations.
func (r *Runner) WithCostControl(controller *costcontrol.Controller) *Runner {
	r.cost = controller
	return r
}

// WithEpochControl enables the P8 kill gate: every dispatch validates the
// episode's RECORDED policy epoch against the control table, so a killed
// epoch refuses in-flight decisions independently of the worker.
func (r *Runner) WithEpochControl(control *runtimecontrol.EpochControl) *Runner {
	r.epochControl = control
	return r
}

// WithShadowStore enables P8 shadow scoring: shadow decisions are scored
// and persisted to shadow_decisions (never to intents/commands).
func (r *Runner) WithShadowStore(store *qualification.ShadowStore) *Runner {
	r.shadowStore = store
	return r
}

// WithTelemetry enables the P8 freshness/latency surface: stale-decision
// rejections and dispatch→decision durations feed the /metrics percentiles.
func (r *Runner) WithTelemetry(telemetry *telemetry.Runtime) *Runner {
	r.telemetry = telemetry
	return r
}

// WithAssembler enables the ISSUE-061 re-bind path: an admitted episode whose
// situation advanced past its bound version is re-bound to the live version
// and dispatched instead of abandoned. Without an assembler the runner keeps
// the pre-fix abandon behavior (tests and minimal wiring).
func (r *Runner) WithAssembler(assembler *Assembler) *Runner {
	r.assembler = assembler
	return r
}

// NewRunner creates a runner for the given executor and clock.
func NewRunner(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator) *Runner {
	return NewRunnerWithEpoch(db, executor, clk, idGen, "")
}

// NewRunnerWithEpoch creates a runner that fences every attempt to ownerEpoch.
// Live runtime composition must use a freshly claimed epoch.
func NewRunnerWithEpoch(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator, ownerEpoch string) *Runner {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Runner{db: db, executor: executor, clk: clk, idGen: idGen, ownerEpoch: ownerEpoch}
}

// RunOnce finds one admitted episode, fences a worker attempt, executes it,
// and persists the attempt terminal state and proposed Decision.
// It returns true if an episode was processed.
func (r *Runner) RunOnce(ctx context.Context, tenantID string) (bool, error) {
	claim, err := r.claimEpisode(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}
	// A quarantined episode was committed inside the claim transaction:
	// report it processed so the batch continues past it.
	if claim.quarantined {
		return true, nil
	}
	return true, r.executeAdmittedClaim(ctx, claim)
}

func (r *Runner) executeAdmittedClaim(ctx context.Context, claim *episodeClaim) error {
	// A re-bind is counted only after its transaction committed. A later
	// in-transaction failure rolls it back and must not bump the counter.
	if claim.rebound && r.telemetry != nil {
		r.telemetry.ObserveStaleRebind()
	}

	startedAt := r.clk.Now()
	outcome, executionErr := r.executeClaim(ctx, claim)
	if r.telemetry != nil {
		r.telemetry.ObserveDuration(r.clk.Now().Sub(startedAt))
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return r.recordExecution(persistCtx, claim, outcome, executionErr, r.deadlineExceeded(&claim.req, startedAt))
}

// executeClaim runs the fenced attempt under a supersession watch, so a
// superseded episode cancels its provider call.
func (r *Runner) executeClaim(ctx context.Context, claim *episodeClaim) (outcome *Outcome, executionErr error) {
	executionCtx, stopWatching := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go r.watchSupersession(executionCtx, claim.episodeID, stopWatching, watchDone)
	defer func() {
		stopWatching()
		<-watchDone
	}()

	return r.executeTracedAttempt(executionCtx, claim)
}

func (r *Runner) executeTracedAttempt(executionCtx context.Context, claim *episodeClaim) (outcome *Outcome, executionErr error) {
	_, span := telemetry.StartSpan(executionCtx, "agentic_stream.episode.execute")
	telemetry.AddLinkFromW3C(span, claim.req.Traceparent, claim.req.Tracestate)
	defer func() {
		if executionErr != nil {
			telemetry.RecordError(span, executionErr)
		}
		span.End()
	}()
	outcome, executionErr = r.executor.Execute(executionCtx, &claim.req)
	if executionErr != nil {
		// Wrapped errors keep their context and budget types; the failure
		// classifiers unwrap with errors.Is/As.
		return nil, fmt.Errorf("execute episode attempt: %w", executionErr)
	}
	return outcome, nil
}

func (r *Runner) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := r.db.WithTx(ctx, fn); err != nil {
		return fmt.Errorf("tx: %w", err)
	}
	return nil
}
