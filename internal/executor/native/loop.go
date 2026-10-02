package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

func (e *Executor) executeBounded(ctx context.Context, req *episodes.Request, payload requestPayload, budget budgetConfig, tools map[string]Tool) (*episodes.Outcome, error) {
	bounded, cancel := context.WithTimeout(ctx, budget.WallTime)
	defer cancel()
	return e.executeLoop(bounded, req, payload, budget, tools)
}

type budgetConfig struct {
	WallTime             time.Duration
	ModelCalls           uint32
	InputTokens          uint64
	OutputTokens         uint64
	ToolCalls            uint32
	ToolResultBytes      uint64
	TotalToolResultBytes uint64
	ProviderRetries      uint32
	CostMicrounits       uint64
}

func (e *Executor) executeLoop(ctx context.Context, req *episodes.Request, payload requestPayload, budget budgetConfig, tools map[string]Tool) (*episodes.Outcome, error) {
	loop := &episodeLoop{
		executor: e, req: req, payload: payload, budget: budget, tools: tools,
		modelReq:  ModelRequest{Episode: req, Prompt: payload.Executor.Prompt, Objective: payload.Executor.Objective, Snapshot: payload.Snapshot, DecisionSchema: payload.Executor.DecisionSchema, AllowedIntentTypes: append([]string(nil), payload.AllowedIntentTypes...), RiskCeiling: payload.RiskCeiling, Tools: toolDefinitions(payload.Tools, tools)},
		seenCalls: make(map[string]struct{}),
	}
	for {
		if outcome := loop.step(ctx); outcome != nil {
			return outcome, nil
		}
	}
}

// episodeLoop is the bounded model/tool loop of one native episode. Each step
// makes one model call and either runs the requested tools, accepts a valid
// Decision, or asks the model to repair an invalid one. Every budget is
// enforced by the loop itself, never by the provider.
type episodeLoop struct {
	executor *Executor
	req      *episodes.Request
	payload  requestPayload
	budget   budgetConfig
	tools    map[string]Tool
	modelReq ModelRequest

	usage                                           Usage
	observations                                    []Observation
	seenCalls                                       map[string]struct{}
	repairs, modelCalls, toolCalls, providerRetries uint32
	toolResultBytes                                 uint64
}

// step runs one model call. It returns the terminal outcome, or nil when the
// loop should continue.
func (l *episodeLoop) step(ctx context.Context) *episodes.Outcome {
	if err := ctx.Err(); err != nil {
		return terminalForContext(l.req, err, l.usage)
	}
	if l.budget.ModelCalls > 0 && l.modelCalls >= l.budget.ModelCalls {
		return failed(l.req, "budget_exhausted:model_calls", l.usage)
	}
	l.modelReq.Observations = append([]Observation(nil), l.observations...)
	l.modelCalls++
	response, err := l.executor.provider.Stream(ctx, l.modelReq)
	if err != nil {
		return l.providerFailure(ctx, err)
	}
	if outcome := l.account(ctx, response); outcome != nil {
		return outcome
	}
	if len(response.ToolCalls) > 0 {
		return l.runTools(ctx, response.ToolCalls)
	}
	return l.decide(response)
}

// providerFailure classifies a failed model call. A retryable failure within
// the retry budget returns nil so the loop calls the model again.
func (l *episodeLoop) providerFailure(ctx context.Context, err error) *episodes.Outcome {
	if errors.Is(err, ErrInterrupt) {
		return failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return terminalForContext(l.req, err, l.usage)
	}
	var retryable *RetryableError
	if !errors.As(err, &retryable) {
		return failed(l.req, "provider_failed", l.usage)
	}
	if l.providerRetries < l.budget.ProviderRetries {
		l.providerRetries++
		if waitErr := waitProviderRetry(ctx); waitErr != nil {
			return terminalForContext(l.req, waitErr, l.usage)
		}
		return nil
	}
	if l.budget.ProviderRetries > 0 {
		return failed(l.req, "budget_exhausted:provider_retries", l.usage)
	}
	return failed(l.req, "provider_failed", l.usage)
}

// account settles the response's usage and enforces the usage budgets and
// wall time.
func (l *episodeLoop) account(ctx context.Context, response ModelResponse) *episodes.Outcome {
	// Count usage before checking cancellation: a provider may return a late
	// response after its context was canceled, but that completed call still
	// consumed provider resources and must be settled.
	l.usage = addUsage(l.usage, response.Usage)
	if hasUsageBudget(l.budget) && !responseUsageReported(response) {
		return failed(l.req, "budget_telemetry_missing", l.usage)
	}
	// A provider is expected to honor cancellation, but a hard wall-time
	// budget must also win when an adapter returns a late response.
	if err := ctx.Err(); err != nil {
		return terminalForContext(l.req, err, l.usage)
	}
	if err := checkUsage(l.usage, l.budget); err != nil {
		return failed(l.req, err.Error(), l.usage)
	}
	return nil
}

// runTools executes the requested tool calls as observations for the next
// model call. Only allow-listed tools with valid, never-repeated arguments
// run, within the tool-call and result-byte budgets.
func (l *episodeLoop) runTools(ctx context.Context, calls []ToolCall) *episodes.Outcome {
	if l.budget.ToolCalls > 0 && uint32(len(calls)) > l.budget.ToolCalls-l.toolCalls { //nolint:gosec // bounded by provider response and checked below.
		return failed(l.req, "budget_exhausted:tool_calls", l.usage)
	}
	for _, call := range calls {
		l.toolCalls++
		if err := ctx.Err(); err != nil {
			return terminalForContext(l.req, err, l.usage)
		}
		if outcome := l.runTool(ctx, call); outcome != nil {
			return outcome
		}
	}
	return nil
}

func (l *episodeLoop) runTool(ctx context.Context, call ToolCall) *episodes.Outcome {
	tool, ok := l.tools[call.Name]
	if !ok {
		return failed(l.req, "tool_not_allowed:"+call.Name, l.usage)
	}
	if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
		return failed(l.req, "tool_arguments_invalid:"+call.Name, l.usage)
	}
	digest := sha256.Sum256(append([]byte(call.Name+"|"), call.Arguments...))
	key := hex.EncodeToString(digest[:])
	if _, exists := l.seenCalls[key]; exists {
		return failed(l.req, "repeated_tool_call:"+call.Name, l.usage)
	}
	l.seenCalls[key] = struct{}{}
	result, err := tool.Call(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, ErrInterrupt) {
			return failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
		}
		l.observations = append(l.observations, Observation{CallID: call.ID, ToolName: call.Name, ErrorCode: "tool_failed"})
		return nil
	}
	observation, err := l.executor.observe(ctx, call, result, l.budget)
	if err != nil {
		return failed(l.req, err.Error(), l.usage)
	}
	l.toolResultBytes += observation.Bytes
	if l.budget.TotalToolResultBytes > 0 && l.toolResultBytes > l.budget.TotalToolResultBytes {
		return failed(l.req, "budget_exhausted:total_tool_result_bytes", l.usage)
	}
	l.observations = append(l.observations, observation)
	return nil
}

// decide accepts a valid Decision as the produced outcome. An invalid one is
// sent back for repair until the repair budget is spent.
func (l *episodeLoop) decide(response ModelResponse) *episodes.Outcome {
	if len(response.DecisionJSON) == 0 {
		return failed(l.req, "provider_returned_no_decision", l.usage)
	}
	decision, validationErr := validateDecision(l.req, response.DecisionJSON, l.payload.AllowedIntentTypes)
	if validationErr == nil {
		return l.produced(decision)
	}
	if l.repairs >= l.executor.maxRepair {
		return failed(l.req, "decision_rejected:"+validationErr.Error(), l.usage)
	}
	l.repairs++
	l.modelReq.Repair = true
	l.modelReq.RepairReason = validationErr.Error()
	return nil
}

func (l *episodeLoop) produced(decision map[string]any) *episodes.Outcome {
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return failed(l.req, "decision_digest_failed", l.usage)
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return failed(l.req, "decision_canonicalization_failed", l.usage)
	}
	return &episodes.Outcome{Status: string(episodeledger.AttemptProduced), AttemptID: l.req.AttemptID, Fence: l.req.Fence, DecisionJSON: decisionJSON, DecisionSHA256: digest, CostMicrounits: l.usage.CostMicrounits}
}

func number(value any) float64 {
	if n, ok := value.(float64); ok {
		return n
	}
	return -1
}

func addUsage(a, b Usage) Usage {
	return Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CostMicrounits: a.CostMicrounits + b.CostMicrounits}
}

func hasUsageBudget(budget budgetConfig) bool {
	return budget.InputTokens > 0 || budget.OutputTokens > 0 || budget.CostMicrounits > 0
}

func responseUsageReported(response ModelResponse) bool {
	return response.UsageReported || response.Usage != (Usage{})
}

func checkUsage(usage Usage, budget budgetConfig) error {
	if budget.InputTokens > 0 && usage.InputTokens > budget.InputTokens {
		return errors.New("budget_exhausted:input_tokens")
	}
	if budget.OutputTokens > 0 && usage.OutputTokens > budget.OutputTokens {
		return errors.New("budget_exhausted:output_tokens")
	}
	if budget.CostMicrounits > 0 && usage.CostMicrounits > budget.CostMicrounits {
		return errors.New("budget_exhausted:cost_microunits")
	}
	return nil
}

const providerRetryBackoff = 10 * time.Millisecond

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

func terminalForContext(req *episodes.Request, err error, usage Usage) *episodes.Outcome {
	if errors.Is(err, context.DeadlineExceeded) {
		return failed(req, "timed_out", usage)
	}
	return &episodes.Outcome{Status: string(episodeledger.AttemptCancelled), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{"canceled"}, CostMicrounits: usage.CostMicrounits}
}

func failed(req *episodes.Request, reason string, usage Usage) *episodes.Outcome {
	return &episodes.Outcome{Status: string(episodeledger.AttemptFailed), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{reason}, CostMicrounits: usage.CostMicrounits}
}
