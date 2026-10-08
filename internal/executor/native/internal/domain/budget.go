package domain

import (
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

type Budget struct {
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

func Number(value any) float64 {
	if n, ok := value.(float64); ok {
		return n
	}
	return -1
}

func AddUsage(a, b Usage) Usage {
	return Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CostMicrounits: a.CostMicrounits + b.CostMicrounits}
}

func HasUsageBudget(budget Budget) bool {
	return budget.InputTokens > 0 || budget.OutputTokens > 0 || budget.CostMicrounits > 0
}

func ResponseUsageReported(response ModelResponse) bool {
	return response.UsageReported || response.Usage != (Usage{})
}

func CheckUsage(usage Usage, budget Budget) error {
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

func Failed(req *episodes.Request, reason string, usage Usage) *episodes.Outcome {
	return &episodes.Outcome{Status: string(episodeledger.AttemptFailed), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{reason}, CostMicrounits: usage.CostMicrounits}
}
