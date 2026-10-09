package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestExecuteFailsTheAttemptWithTheReasonOfTheBoundItBreaches(t *testing.T) {
	t.Parallel()
	noUsage := func(_ int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		response, err := validDecision(ctx, req)
		response.Usage, response.UsageReported = native.Usage{}, false
		return response, err
	}
	endlessTools := func(call int, _ context.Context, _ native.ModelRequest) (native.ModelResponse, error) {
		return native.ModelResponse{ToolCalls: []native.ToolCall{toolCall("evidence.get", fmt.Sprintf(`{"n":%d}`, call))}}, nil
	}
	foreignDecision := func(int, context.Context, native.ModelRequest) (native.ModelResponse, error) {
		return native.ModelResponse{DecisionJSON: []byte(`{"episode_id":"other"}`)}, nil
	}
	retryable := &native.RetryableError{Err: errors.New("temporary")}
	tests := []struct {
		name      string
		respond   respondFunc
		tools     []native.Tool
		budget    map[string]any
		wantCalls int
		want      string
	}{
		{"interrupt", failingWith(native.ErrInterrupt), nil, map[string]any{"model_calls": 3}, 1, "interrupt_in_non_interactive_episode"},
		{"input tokens", withUsage(native.Usage{InputTokens: 11}), nil, map[string]any{"input_tokens": 10, "model_calls": 3}, 1, "budget_exhausted:input_tokens"},
		{"output tokens", withUsage(native.Usage{OutputTokens: 11}), nil, map[string]any{"output_tokens": 10, "model_calls": 3}, 1, "budget_exhausted:output_tokens"},
		{"cost", withUsage(native.Usage{CostMicrounits: 11}), nil, map[string]any{"cost_microunits": 10, "model_calls": 3}, 1, "budget_exhausted:cost_microunits"},
		{"missing usage telemetry", noUsage, nil, map[string]any{"input_tokens": 10, "model_calls": 1}, 1, "budget_telemetry_missing"},
		{"provider retries", failingWith(retryable), nil, map[string]any{"model_calls": 5, "provider_retries": 1}, 2, "budget_exhausted:provider_retries"},
		{"retryable error with no retry allowance", failingWith(retryable), nil, map[string]any{"model_calls": 5}, 1, "provider_failed"},
		{"provider failure", failingWith(errors.New("boom")), nil, map[string]any{"model_calls": 5, "provider_retries": 3}, 1, "provider_failed"},
		{"model calls", endlessTools, []native.Tool{toolReturning("evidence.get", `{}`)}, map[string]any{"model_calls": 2}, 2, "budget_exhausted:model_calls"},
		{"no decision", func(int, context.Context, native.ModelRequest) (native.ModelResponse, error) {
			return native.ModelResponse{}, nil
		}, nil, map[string]any{"model_calls": 3}, 1, "provider_returned_no_decision"},
		{"decision rejected after the repair attempt", foreignDecision, nil, map[string]any{"model_calls": 5}, 2, "decision_rejected:decision_identity_mismatch"},
		{"unlisted tool", callingTools(toolCall("ghost", `{}`)), nil, map[string]any{"model_calls": 3}, 1, "tool_not_allowed:ghost"},
		{"tool arguments that are not json", callingTools(toolCall("evidence.get", `{broken`)), []native.Tool{toolReturning("evidence.get", `{}`)}, map[string]any{"model_calls": 3}, 1, "tool_arguments_invalid:evidence.get"},
		{"tool call without arguments", callingTools(toolCall("evidence.get", ``)), []native.Tool{toolReturning("evidence.get", `{}`)}, map[string]any{"model_calls": 3}, 1, "tool_arguments_invalid:evidence.get"},
		{"tool calls", callingTools(toolCall("evidence.get", `{"a":1}`), toolCall("evidence.get", `{"a":2}`)), []native.Tool{toolReturning("evidence.get", `{}`)}, map[string]any{"model_calls": 3, "tool_calls": 1}, 1, "budget_exhausted:tool_calls"},
		{"tool result bytes", callingTools(toolCall("evidence.get", `{}`)), []native.Tool{toolReturning("evidence.get", `{"large":"payload"}`)}, map[string]any{"model_calls": 3, "tool_result_bytes": 4}, 1, "tool_result_oversized:evidence.get"},
		{"tool result that is not json", callingTools(toolCall("evidence.get", `{}`)), []native.Tool{toolReturning("evidence.get", `{broken`)}, map[string]any{"model_calls": 3}, 1, "tool_result_invalid:evidence.get"},
		{"tool interrupt", callingTools(toolCall("evidence.get", `{}`)), []native.Tool{toolFunc{name: "evidence.get", call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
			return native.ToolResult{}, native.ErrInterrupt
		}}}, map[string]any{"model_calls": 3}, 1, "interrupt_in_non_interactive_episode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider := providerRespondingWith(tt.respond)
			outcome := execute(t, newExecutor(t, provider, tt.tools...), tt.budget)
			if outcome.Status != string(episodeledger.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != tt.want {
				t.Fatalf("Execute() outcome = %+v, want a failed attempt with reason %q", outcome, tt.want)
			}
			if provider.calls() != tt.wantCalls {
				t.Fatalf("provider calls = %d, want %d", provider.calls(), tt.wantCalls)
			}
			if len(outcome.DecisionJSON) != 0 {
				t.Fatalf("a failed attempt carried a decision: %s", outcome.DecisionJSON)
			}
		})
	}
}

func TestExecuteRefusesAnUnboundedRequestWithoutCallingTheProvider(t *testing.T) {
	t.Parallel()
	provider := providerRespondingWith(decidesImmediately)
	_, err := newExecutor(t, provider).Execute(t.Context(), requestWithBudget(map[string]any{}))
	if err == nil || err.Error() != "finite episode budget requires wall_time or model_calls" {
		t.Fatalf("Execute() = %v, want the finite-budget refusal", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider calls = %d, want 0 for an unbounded request", provider.calls())
	}
}
