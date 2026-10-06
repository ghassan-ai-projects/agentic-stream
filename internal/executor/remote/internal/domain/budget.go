package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// budgetUsage is the runtime's trusted count of what a worker has consumed.
// It takes the maximum of event-derived counts and worker-reported totals, so
// a worker cannot under-report its way past a limit.
type budgetUsage struct {
	modelCalls, toolCalls, providerRetries uint32
	toolResultBytes                        uint64
	perEventUsage, cumulativeUsage         usageTotals
	hasCumulativeUsage, usageReported      bool
}

type usageTotals struct {
	inputTokens, outputTokens, costMicrounits uint64
}

func (u *budgetUsage) observe(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if limit == nil || event == nil {
		return nil
	}
	if err := u.observeModel(limit, event); err != nil {
		return err
	}
	if err := u.observeBudgetEvent(limit, event.GetBudget()); err != nil {
		return err
	}
	if err := u.observeTools(limit, event); err != nil {
		return err
	}
	return u.checkUsage(limit)
}

func (u *budgetUsage) observeModel(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if event.GetModelStarted() != nil {
		u.modelCalls++
		if limit.GetMaxModelCalls() > 0 && u.modelCalls > limit.GetMaxModelCalls() {
			return &episodes.BudgetExceededError{Metric: "model_calls"}
		}
	}
	if completed := event.GetModelCompleted(); completed != nil && completed.GetUsage() != nil {
		u.addPerEventUsage(completed.GetUsage())
	}
	return nil
}

func (u *budgetUsage) addPerEventUsage(usage *runtimev1.Usage) {
	u.usageReported = true
	values := usageFromProto(usage)
	u.perEventUsage.inputTokens += values.inputTokens
	u.perEventUsage.outputTokens += values.outputTokens
	u.perEventUsage.costMicrounits += values.costMicrounits
}

// observeBudgetEvent records a worker budget update and its cumulative usage.
func (u *budgetUsage) observeBudgetEvent(limit *runtimev1.EpisodeBudget, budget *runtimev1.BudgetUpdated) error {
	if budget == nil {
		return nil
	}
	if budget.GetCumulativeUsage() != nil {
		if err := u.recordCumulativeUsage(budget.GetCumulativeUsage()); err != nil {
			return err
		}
	}
	return u.observeBudgetUpdate(limit, budget)
}

func (u *budgetUsage) recordCumulativeUsage(usage *runtimev1.Usage) error {
	u.usageReported = true
	values := usageFromProto(usage)
	if u.hasCumulativeUsage && (values.inputTokens < u.cumulativeUsage.inputTokens || values.outputTokens < u.cumulativeUsage.outputTokens || values.costMicrounits < u.cumulativeUsage.costMicrounits) {
		return fmt.Errorf("worker cumulative usage regressed")
	}
	u.cumulativeUsage = values
	u.hasCumulativeUsage = true
	return nil
}

func (u *budgetUsage) observeBudgetUpdate(limit *runtimev1.EpisodeBudget, update *runtimev1.BudgetUpdated) error {
	if limit == nil || update == nil {
		return nil
	}
	u.modelCalls = max(u.modelCalls, update.GetModelCallsUsed())
	u.toolCalls = max(u.toolCalls, update.GetToolCallsUsed())
	u.toolResultBytes = max(u.toolResultBytes, update.GetToolResultBytesUsed())
	u.providerRetries = max(u.providerRetries, update.GetProviderRetriesUsed())
	return checkReportedBudget(limit, update)
}

func checkReportedBudget(limit *runtimev1.EpisodeBudget, update *runtimev1.BudgetUpdated) error {
	if limit.GetMaxModelCalls() > 0 && update.GetModelCallsUsed() > limit.GetMaxModelCalls() {
		return &episodes.BudgetExceededError{Metric: "model_calls"}
	}
	if limit.GetMaxToolCalls() > 0 && update.GetToolCallsUsed() > limit.GetMaxToolCalls() {
		return &episodes.BudgetExceededError{Metric: "tool_calls"}
	}
	return checkReportedToolLimits(limit, update)
}

func checkReportedToolLimits(limit *runtimev1.EpisodeBudget, update *runtimev1.BudgetUpdated) error {
	if limit.GetMaxToolResultBytes() > 0 && update.GetToolResultBytesUsed() > limit.GetMaxToolResultBytes() {
		return &episodes.BudgetExceededError{Metric: "tool_result_bytes"}
	}
	if limit.GetMaxProviderRetries() > 0 && update.GetProviderRetriesUsed() > limit.GetMaxProviderRetries() {
		return &episodes.BudgetExceededError{Metric: "provider_retries"}
	}
	if usage := update.GetCumulativeUsage(); usage != nil {
		return checkUsageLimits(limit, usageFromProto(usage))
	}
	return nil
}

func (u *budgetUsage) observeTools(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if tool := event.GetTool(); tool != nil && tool.GetExecutionStarted() {
		u.toolCalls++
		if limit.GetMaxToolCalls() > 0 && u.toolCalls > limit.GetMaxToolCalls() {
			return &episodes.BudgetExceededError{Metric: "tool_calls"}
		}
	}
	return u.observeToolProgress(limit, event)
}

func (u *budgetUsage) observeToolProgress(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if progress := event.GetToolProgress(); progress != nil {
		u.toolResultBytes += progress.GetBytesRead()
		if limit.GetMaxToolResultBytes() > 0 && u.toolResultBytes > limit.GetMaxToolResultBytes() {
			return &episodes.BudgetExceededError{Metric: "tool_result_bytes"}
		}
		if limit.GetMaxTotalToolResultBytes() > 0 && u.toolResultBytes > limit.GetMaxTotalToolResultBytes() {
			return &episodes.BudgetExceededError{Metric: "total_tool_result_bytes"}
		}
	}
	return nil
}

func (u *budgetUsage) checkUsage(limit *runtimev1.EpisodeBudget) error {
	if limit == nil {
		return nil
	}
	return checkUsageLimits(limit, u.usage())
}

func (u *budgetUsage) usage() usageTotals {
	result := u.perEventUsage
	if u.hasCumulativeUsage {
		result.inputTokens = max(result.inputTokens, u.cumulativeUsage.inputTokens)
		result.outputTokens = max(result.outputTokens, u.cumulativeUsage.outputTokens)
		result.costMicrounits = max(result.costMicrounits, u.cumulativeUsage.costMicrounits)
	}
	return result
}

func usageFromProto(usage *runtimev1.Usage) usageTotals {
	if usage == nil {
		return usageTotals{}
	}
	return usageTotals{inputTokens: usage.GetInputTokens(), outputTokens: usage.GetOutputTokens(), costMicrounits: usage.GetCostMicrounits()}
}

// checkUsageLimits reports the first token or cost limit that usage exceeds.
func checkUsageLimits(limit *runtimev1.EpisodeBudget, usage usageTotals) error {
	if limit.GetMaxInputTokens() > 0 && usage.inputTokens > limit.GetMaxInputTokens() {
		return &episodes.BudgetExceededError{Metric: "input_tokens"}
	}
	if limit.GetMaxOutputTokens() > 0 && usage.outputTokens > limit.GetMaxOutputTokens() {
		return &episodes.BudgetExceededError{Metric: "output_tokens"}
	}
	if limit.GetMaxCostMicrounits() > 0 && usage.costMicrounits > limit.GetMaxCostMicrounits() {
		return &episodes.BudgetExceededError{Metric: "cost_microunits"}
	}
	return nil
}

func (u *budgetUsage) observeUsage(limit *runtimev1.EpisodeBudget, usage *runtimev1.Usage) error {
	if err := u.recordCumulativeUsage(usage); err != nil {
		return err
	}
	return u.checkUsage(limit)
}

func hasUsageBudget(budget *runtimev1.EpisodeBudget) bool {
	return budget != nil && (budget.GetMaxInputTokens() > 0 || budget.GetMaxOutputTokens() > 0 || budget.GetMaxCostMicrounits() > 0)
}

func hasNumericBudget(budget *runtimev1.EpisodeBudget) bool {
	return budget != nil && (budget.GetMaxModelCalls() > 0 || budget.GetMaxInputTokens() > 0 || budget.GetMaxOutputTokens() > 0 || budget.GetMaxToolCalls() > 0 || budget.GetMaxToolResultBytes() > 0 || budget.GetMaxTotalToolResultBytes() > 0 || budget.GetMaxProviderRetries() > 0 || budget.GetMaxCostMicrounits() > 0)
}
