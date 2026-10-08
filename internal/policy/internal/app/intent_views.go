package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// Intent reads one of the tenant's intents with its evaluations.
func Intent(ctx context.Context, reader store.Reader, tenantID, intentID string) (domain.IntentView, error) {
	return reader.Intent(ctx, tenantID, intentID) //nolint:wrapcheck // The store names the failed read.
}

// DecisionIntents reads the intents one Decision proposed.
func DecisionIntents(ctx context.Context, reader store.Reader, tenantID, decisionID string) ([]domain.IntentView, error) {
	return reader.DecisionIntents(ctx, tenantID, decisionID) //nolint:wrapcheck // The store names the failed read.
}
