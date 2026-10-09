package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

func TestCheckUsageNamesTheExhaustedBudget(t *testing.T) {
	t.Parallel()
	budget := Budget{InputTokens: 10, OutputTokens: 10, CostMicrounits: 10}
	tests := []struct {
		name   string
		usage  Usage
		budget Budget
		want   string
	}{
		{"input tokens", Usage{InputTokens: 11}, budget, "budget_exhausted:input_tokens"},
		{"output tokens", Usage{OutputTokens: 11}, budget, "budget_exhausted:output_tokens"},
		{"cost", Usage{CostMicrounits: 11}, budget, "budget_exhausted:cost_microunits"},
		{"usage at the ceiling", Usage{InputTokens: 10, OutputTokens: 10, CostMicrounits: 10}, budget, ""},
		{"no budget", Usage{InputTokens: 1 << 40}, Budget{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckUsage(tt.usage, tt.budget)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Fatalf("CheckUsage(%+v) = %v, want %q", tt.usage, err, tt.want)
			}
		})
	}
}

func TestUsageAccounting(t *testing.T) {
	t.Parallel()
	if got := AddUsage(Usage{InputTokens: 1, CostMicrounits: 2}, Usage{InputTokens: 3, OutputTokens: 4}); got != (Usage{InputTokens: 4, OutputTokens: 4, CostMicrounits: 2}) {
		t.Fatalf("AddUsage() = %+v", got)
	}
	for budget, want := range map[Budget]bool{
		{}: false, {ModelCalls: 3}: false, {InputTokens: 1}: true, {OutputTokens: 1}: true, {CostMicrounits: 1}: true,
	} {
		if got := HasUsageBudget(budget); got != want {
			t.Errorf("HasUsageBudget(%+v) = %v, want %v", budget, got, want)
		}
	}
	for _, c := range []struct {
		name     string
		response ModelResponse
		want     bool
	}{
		{"nothing reported", ModelResponse{}, false},
		{"flagged as reported", ModelResponse{UsageReported: true}, true},
		{"non-zero usage", ModelResponse{Usage: Usage{InputTokens: 1}}, true},
	} {
		if got := ResponseUsageReported(c.response); got != c.want {
			t.Errorf("ResponseUsageReported(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFailedOutcomeBindsAttemptIdentityAndCost(t *testing.T) {
	t.Parallel()
	req := &episodes.Request{AttemptID: "att", Fence: 3}
	failed := Failed(req, "why", Usage{CostMicrounits: 7})
	if failed.Status != "failed" || failed.AttemptID != "att" || failed.Fence != 3 || len(failed.Reasons) != 1 || failed.Reasons[0] != "why" || failed.CostMicrounits != 7 {
		t.Fatalf("Failed() = %+v", failed)
	}
}

func TestRequestBudgetRequiresAFiniteBound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		budget  map[string]any
		want    Budget
		wantErr string
	}{
		{"wall time", map[string]any{"wall_time": "5s"}, Budget{WallTime: 5 * time.Second}, ""},
		{"model calls", map[string]any{"model_calls": 3}, Budget{ModelCalls: 3}, ""},
		{"every ceiling", map[string]any{
			"wall_time": "1m", "model_calls": 4, "input_tokens": 100, "output_tokens": 50, "tool_calls": 3, "tool_result_bytes": 1024,
			"total_tool_result_bytes": 4096, "provider_retries": 2, "cost_microunits": 900,
		}, Budget{WallTime: time.Minute, ModelCalls: 4, InputTokens: 100, OutputTokens: 50, ToolCalls: 3, ToolResultBytes: 1024, TotalToolResultBytes: 4096, ProviderRetries: 2, CostMicrounits: 900}, ""},
		{"neither wall time nor model calls", map[string]any{"tool_calls": 3}, Budget{}, "finite episode budget requires wall_time or model_calls"},
		{"no budget", map[string]any{}, Budget{}, "finite episode budget requires wall_time or model_calls"},
		{"malformed wall time", map[string]any{"wall_time": "soon"}, Budget{}, "validate episode budget"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := executorconformance.FixtureRequest()
			executorconformance.SetBudget(req, tt.budget)
			payload, err := DecodeRequest(req.RequestJSON)
			if err != nil {
				t.Fatal(err)
			}
			got, err := RequestBudget(req, payload)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("RequestBudget() = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("RequestBudget() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
