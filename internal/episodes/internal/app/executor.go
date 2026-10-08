package app

import (
	"context"
	"fmt"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Runner polls admitted episodes and executes them deterministically.
type Runner struct {
	episodes      store.Store
	executor      Executor
	clk           sources.Clock
	idGen         sources.Generator
	ownerEpoch    string
	cost          *runtimecontrol.CostLedger
	decisionEpoch store.OwnerCheck
	telemetry     *telemetry.Runtime
	assembler     *Assembler
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
	persistCtx, cancel := sources.DetachedContext(ctx)
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

func (r *Runner) withTx(ctx context.Context, fn func(*store.Tx) error) error {
	if err := r.episodes.WithTx(ctx, fn); err != nil {
		return fmt.Errorf("tx: %w", err)
	}
	return nil
}
