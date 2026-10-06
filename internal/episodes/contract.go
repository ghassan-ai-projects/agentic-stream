package episodes

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Request is the immutable situation-bound input to a bounded episode.
type Request = domain.Request

// Outcome is the terminal result of one fenced worker attempt.
type Outcome = domain.Outcome

// Executor returns proposals; it must never mutate stream or action state.
type Executor = domain.Executor

// BudgetExceededError identifies consumption beyond an admitted metric budget.
type BudgetExceededError = domain.BudgetExceededError

// BudgetTelemetryMissingError identifies an attempt without proven usage telemetry.
type BudgetTelemetryMissingError = domain.BudgetTelemetryMissingError

// CompileIntentCatalog binds the spec's intent authority to its canonical digest.
func CompileIntentCatalog(intents []spec.Intent) ([]map[string]any, string, error) {
	return domain.CompileIntentCatalog(intents)
}
