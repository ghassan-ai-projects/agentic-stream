package domain

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestBudgetUsageNamesTheMetricAWorkerExceeds(t *testing.T) {
	t.Parallel()
	usage := func(input, output, cost uint64) *runtimev1.Usage {
		return &runtimev1.Usage{InputTokens: input, OutputTokens: output, CostMicrounits: cost}
	}
	tests := []struct {
		name       string
		limit      *runtimev1.EpisodeBudget
		events     []*runtimev1.EpisodeEvent
		wantMetric string
	}{
		{"second model call", &runtimev1.EpisodeBudget{MaxModelCalls: 1}, []*runtimev1.EpisodeEvent{modelStarted(), modelStarted()}, "model_calls"},
		{"reported model calls", &runtimev1.EpisodeBudget{MaxModelCalls: 1}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{ModelCallsUsed: 2})}, "model_calls"},
		{"second tool execution", &runtimev1.EpisodeBudget{MaxToolCalls: 1}, []*runtimev1.EpisodeEvent{toolExecutionStarted(), toolExecutionStarted()}, "tool_calls"},
		{"reported tool calls", &runtimev1.EpisodeBudget{MaxToolCalls: 1}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{ToolCallsUsed: 2})}, "tool_calls"},
		{"tool result bytes read", &runtimev1.EpisodeBudget{MaxToolResultBytes: 10}, []*runtimev1.EpisodeEvent{toolProgress(6), toolProgress(5)}, "tool_result_bytes"},
		{"reported tool result bytes", &runtimev1.EpisodeBudget{MaxToolResultBytes: 10}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{ToolResultBytesUsed: 11})}, "tool_result_bytes"},
		{"total tool result bytes", &runtimev1.EpisodeBudget{MaxTotalToolResultBytes: 10}, []*runtimev1.EpisodeEvent{toolProgress(6), toolProgress(5)}, "total_tool_result_bytes"},
		{"reported provider retries", &runtimev1.EpisodeBudget{MaxProviderRetries: 1}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{ProviderRetriesUsed: 2})}, "provider_retries"},
		{"input tokens of a model call", &runtimev1.EpisodeBudget{MaxInputTokens: 10}, []*runtimev1.EpisodeEvent{modelCompleted(usage(6, 0, 0)), modelCompleted(usage(5, 0, 0))}, "input_tokens"},
		{"output tokens of a model call", &runtimev1.EpisodeBudget{MaxOutputTokens: 10}, []*runtimev1.EpisodeEvent{modelCompleted(usage(0, 11, 0))}, "output_tokens"},
		{"cost of a model call", &runtimev1.EpisodeBudget{MaxCostMicrounits: 10}, []*runtimev1.EpisodeEvent{modelCompleted(usage(0, 0, 11))}, "cost_microunits"},
		{"cumulative input tokens", &runtimev1.EpisodeBudget{MaxInputTokens: 10}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{CumulativeUsage: usage(11, 0, 0)})}, "input_tokens"},
		{"cumulative cost", &runtimev1.EpisodeBudget{MaxCostMicrounits: 10}, []*runtimev1.EpisodeEvent{budgetUpdate(&runtimev1.BudgetUpdated{CumulativeUsage: usage(0, 0, 11)})}, "cost_microunits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var counted budgetUsage
			var err error
			for index, event := range tt.events {
				err = counted.observe(tt.limit, event)
				if err != nil && index < len(tt.events)-1 {
					t.Fatalf("event %d rejected early: %v", index+1, err)
				}
			}
			var exceeded *episodes.BudgetExceededError
			if !errors.As(err, &exceeded) || exceeded.Metric != tt.wantMetric {
				t.Fatalf("observe() = %v, want a budget error for %s", err, tt.wantMetric)
			}
		})
	}
}

func TestBudgetUsageAcceptsConsumptionAtTheCeiling(t *testing.T) {
	t.Parallel()
	limit := &runtimev1.EpisodeBudget{MaxModelCalls: 2, MaxToolCalls: 1, MaxToolResultBytes: 10, MaxInputTokens: 10, MaxCostMicrounits: 10}
	events := []*runtimev1.EpisodeEvent{
		modelStarted(), modelStarted(), toolExecutionStarted(), toolProgress(10),
		modelCompleted(&runtimev1.Usage{InputTokens: 10, CostMicrounits: 10}),
		budgetUpdate(&runtimev1.BudgetUpdated{ModelCallsUsed: 2, ToolCallsUsed: 1, ToolResultBytesUsed: 10, CumulativeUsage: &runtimev1.Usage{InputTokens: 10, CostMicrounits: 10}}),
	}
	var counted budgetUsage
	for index, event := range events {
		if err := counted.observe(limit, event); err != nil {
			t.Fatalf("event %d at the ceiling rejected: %v", index+1, err)
		}
	}
}

func TestBudgetUsageEnforcesNothingWithoutALimit(t *testing.T) {
	t.Parallel()
	events := []*runtimev1.EpisodeEvent{
		modelStarted(), modelStarted(), toolExecutionStarted(), toolProgress(1 << 40),
		modelCompleted(&runtimev1.Usage{InputTokens: 1 << 40}),
	}
	for name, limit := range map[string]*runtimev1.EpisodeBudget{"nil budget": nil, "zero ceilings": {}} {
		var counted budgetUsage
		for _, event := range events {
			if err := counted.observe(limit, event); err != nil {
				t.Errorf("%s: observe() = %v", name, err)
			}
		}
	}
	var counted budgetUsage
	if err := counted.observe(&runtimev1.EpisodeBudget{MaxModelCalls: 1}, nil); err != nil {
		t.Errorf("a nil event must not be counted: %v", err)
	}
}

func TestBudgetUsageKeepsTheWorkersLowerReportsFromReducingTheCount(t *testing.T) {
	t.Parallel()
	limit := &runtimev1.EpisodeBudget{MaxModelCalls: 2}
	var counted budgetUsage
	events := []*runtimev1.EpisodeEvent{modelStarted(), modelStarted(), budgetUpdate(&runtimev1.BudgetUpdated{ModelCallsUsed: 0})}
	for _, event := range events {
		if err := counted.observe(limit, event); err != nil {
			t.Fatal(err)
		}
	}
	var exceeded *episodes.BudgetExceededError
	if err := counted.observe(limit, modelStarted()); !errors.As(err, &exceeded) || exceeded.Metric != "model_calls" {
		t.Fatalf("a worker under-reporting its model calls escaped the ceiling: %v", err)
	}
}

func TestBudgetUsageRefusesCumulativeUsageThatGoesDown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		first *runtimev1.Usage
		next  *runtimev1.Usage
		fails bool
	}{
		{"input tokens", &runtimev1.Usage{InputTokens: 5}, &runtimev1.Usage{InputTokens: 4}, true},
		{"output tokens", &runtimev1.Usage{OutputTokens: 5}, &runtimev1.Usage{OutputTokens: 4}, true},
		{"cost", &runtimev1.Usage{CostMicrounits: 5}, &runtimev1.Usage{CostMicrounits: 4}, true},
		{"unchanged", &runtimev1.Usage{CostMicrounits: 5}, &runtimev1.Usage{CostMicrounits: 5}, false},
		{"growing", &runtimev1.Usage{CostMicrounits: 5}, &runtimev1.Usage{CostMicrounits: 6}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var counted budgetUsage
			limit := &runtimev1.EpisodeBudget{}
			if err := counted.observe(limit, budgetUpdate(&runtimev1.BudgetUpdated{CumulativeUsage: tt.first})); err != nil {
				t.Fatal(err)
			}
			err := counted.observe(limit, budgetUpdate(&runtimev1.BudgetUpdated{CumulativeUsage: tt.next}))
			if tt.fails && (err == nil || err.Error() != "worker cumulative usage regressed") || !tt.fails && err != nil {
				t.Fatalf("observe() = %v, want regression=%v", err, tt.fails)
			}
		})
	}
}
