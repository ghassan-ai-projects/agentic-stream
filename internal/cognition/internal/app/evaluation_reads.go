package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

// TriggerEvaluation reads one of the tenant's trigger evaluations.
func TriggerEvaluation(ctx context.Context, reader store.Reader, tenantID, triggerID string) (domain.TriggerEvaluationRecord, error) {
	return reader.TriggerEvaluation(ctx, tenantID, triggerID) //nolint:wrapcheck // The store names the failed read.
}

// TriggerEvaluations reads every trigger evaluation of one Situation version.
func TriggerEvaluations(ctx context.Context, reader store.Reader, tenantID, situationID string, version int) ([]domain.TriggerEvaluationRecord, error) {
	return reader.TriggerEvaluations(ctx, tenantID, situationID, version) //nolint:wrapcheck // The store names the failed read.
}
