package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
)

// episodeLoop is the bounded model/tool loop of one native episode. Each step
// makes one model call and either runs the requested tools, accepts a valid
// Decision, or asks the model to repair an invalid one. Every budget is
// enforced by the loop itself, never by the provider.
type episodeLoop struct {
	executor *Executor
	req      *episodes.Request
	payload  domain.RequestPayload
	budget   domain.Budget
	tools    map[string]domain.Tool
	modelReq domain.ModelRequest

	usage                                           domain.Usage
	observations                                    []domain.Observation
	seenCalls                                       map[string]struct{}
	repairs, modelCalls, toolCalls, providerRetries uint32
	toolResultBytes                                 uint64
}

const providerRetryBackoff = 10 * time.Millisecond

func (e *Executor) executeBounded(ctx context.Context, req *episodes.Request, payload domain.RequestPayload, budget domain.Budget, tools map[string]domain.Tool) (*episodes.Outcome, error) {
	bounded, cancel := context.WithTimeout(ctx, budget.WallTime)
	defer cancel()
	return e.executeLoop(bounded, req, payload, budget, tools)
}

func (e *Executor) executeLoop(ctx context.Context, req *episodes.Request, payload domain.RequestPayload, budget domain.Budget, tools map[string]domain.Tool) (*episodes.Outcome, error) {
	loop := &episodeLoop{
		executor: e, req: req, payload: payload, budget: budget, tools: tools,
		modelReq:  domain.ModelRequest{Episode: req, Prompt: payload.Executor.Prompt, Objective: payload.Executor.Objective, Snapshot: payload.Snapshot, DecisionSchema: payload.Executor.DecisionSchema, AllowedIntentTypes: append([]string(nil), payload.AllowedIntentTypes...), RiskCeiling: payload.RiskCeiling, Tools: domain.ToolDefinitions(payload.Tools, tools)},
		seenCalls: make(map[string]struct{}),
	}
	for {
		if outcome := loop.step(ctx); outcome != nil {
			return outcome, nil
		}
	}
}

// step runs one model call. It returns the terminal outcome, or nil when the
// loop should continue.
func (l *episodeLoop) step(ctx context.Context) *episodes.Outcome {
	if err := ctx.Err(); err != nil {
		return domain.TerminalForContext(l.req, err, l.usage)
	}
	if l.budget.ModelCalls > 0 && l.modelCalls >= l.budget.ModelCalls {
		return domain.Failed(l.req, "budget_exhausted:model_calls", l.usage)
	}
	l.modelReq.Observations = append([]domain.Observation(nil), l.observations...)
	l.modelCalls++
	response, err := l.executor.provider.Stream(ctx, l.modelReq)
	if err != nil {
		return l.providerFailure(ctx, err)
	}
	return l.acceptResponse(ctx, response)
}

func (l *episodeLoop) acceptResponse(ctx context.Context, response domain.ModelResponse) *episodes.Outcome {
	if outcome := l.account(ctx, response); outcome != nil {
		return outcome
	}
	if len(response.ToolCalls) > 0 {
		return l.runTools(ctx, response.ToolCalls)
	}
	return l.decide(response)
}

// providerFailure classifies a domain.Failed model call. A retryable failure within
// the retry budget returns nil so the loop calls the model again.
func (l *episodeLoop) providerFailure(ctx context.Context, err error) *episodes.Outcome {
	if errors.Is(err, domain.ErrInterrupt) {
		return domain.Failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domain.TerminalForContext(l.req, err, l.usage)
	}
	var retryable *domain.RetryableError
	if !errors.As(err, &retryable) {
		return domain.Failed(l.req, "provider_failed", l.usage)
	}
	return l.retryProvider(ctx)
}

func (l *episodeLoop) retryProvider(ctx context.Context) *episodes.Outcome {
	if l.providerRetries < l.budget.ProviderRetries {
		l.providerRetries++
		if waitErr := waitProviderRetry(ctx); waitErr != nil {
			return domain.TerminalForContext(l.req, waitErr, l.usage)
		}
		return nil
	}
	if l.budget.ProviderRetries > 0 {
		return domain.Failed(l.req, "budget_exhausted:provider_retries", l.usage)
	}
	return domain.Failed(l.req, "provider_failed", l.usage)
}

// account settles the response's usage and enforces the usage budgets and
// wall time.
func (l *episodeLoop) account(ctx context.Context, response domain.ModelResponse) *episodes.Outcome {
	// Count usage before checking cancellation: a provider may return a late
	// response after its context was canceled, but that completed call still
	// consumed provider resources and must be settled.
	l.usage = domain.AddUsage(l.usage, response.Usage)
	if domain.HasUsageBudget(l.budget) && !domain.ResponseUsageReported(response) {
		return domain.Failed(l.req, "budget_telemetry_missing", l.usage)
	}
	// A provider is expected to honor cancellation, but a hard wall-time
	// budget must also win when an adapter returns a late response.
	if err := ctx.Err(); err != nil {
		return domain.TerminalForContext(l.req, err, l.usage)
	}
	if err := domain.CheckUsage(l.usage, l.budget); err != nil {
		return domain.Failed(l.req, err.Error(), l.usage)
	}
	return nil
}

// decide accepts a valid Decision as the produced outcome. An invalid one is
// sent back for repair until the repair budget is spent.
func (l *episodeLoop) decide(response domain.ModelResponse) *episodes.Outcome {
	if len(response.DecisionJSON) == 0 {
		return domain.Failed(l.req, "provider_returned_no_decision", l.usage)
	}
	decision, validationErr := domain.ValidateDecision(l.req, response.DecisionJSON, l.payload.AllowedIntentTypes)
	if validationErr == nil {
		return l.produced(decision)
	}
	if l.repairs >= l.executor.maxRepair {
		return domain.Failed(l.req, "decision_rejected:"+validationErr.Error(), l.usage)
	}
	l.repairs++
	l.modelReq.Repair = true
	l.modelReq.RepairReason = validationErr.Error()
	return nil
}

func (l *episodeLoop) produced(decision map[string]any) *episodes.Outcome {
	decisionJSON, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, decision)
	if err != nil {
		return domain.Failed(l.req, "decision_canonicalization_failed", l.usage)
	}
	return &episodes.Outcome{Status: string(episodeledger.AttemptProduced), AttemptID: l.req.AttemptID, Fence: l.req.Fence, DecisionJSON: decisionJSON, DecisionSHA256: canonicaljson.EncodeDigest(sum), CostMicrounits: l.usage.CostMicrounits}
}

func waitProviderRetry(ctx context.Context) error {
	timer := time.NewTimer(providerRetryBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("provider retry backoff canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
