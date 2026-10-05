package domain

import (
	"context"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// BudgetExceededError reports that an executor stopped an attempt because it
// consumed more than its admitted budget for Metric.
type BudgetExceededError struct{ Metric string }

func (e *BudgetExceededError) Error() string { return "episode budget exceeded: " + e.Metric }

// BudgetTelemetryMissingError reports that an executor could not prove an
// attempt stayed within budget because the usage telemetry was absent.
type BudgetTelemetryMissingError struct{}

func (BudgetTelemetryMissingError) Error() string { return "episode budget telemetry is missing" }

// ExecutionFailureReason classifies an execution error into the durable
// protocol reason recorded with the failed attempt.
func ExecutionFailureReason(err error) string {
	var budgetErr *BudgetExceededError
	if errors.As(err, &budgetErr) {
		return "budget_exhausted"
	}
	var telemetryErr BudgetTelemetryMissingError
	if errors.As(err, &telemetryErr) {
		return "budget_telemetry_missing"
	}
	switch ExecutionFailureStatus(err) {
	case episodeledger.AttemptCancelled:
		return "worker_cancelled" //nolint:misspell // Durable protocol reason is frozen as cancelled.
	case episodeledger.AttemptTimedOut:
		return "worker_deadline_exceeded"
	default:
		return "worker_execution_failed"
	}
}

// ExecutionFailureStatus classifies an execution error into the attempt
// lifecycle status the ledger records.
func ExecutionFailureStatus(err error) episodeledger.AttemptStatus {
	if errors.Is(err, context.Canceled) {
		return episodeledger.AttemptCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return episodeledger.AttemptTimedOut
	}
	return episodeledger.AttemptFailed
}
