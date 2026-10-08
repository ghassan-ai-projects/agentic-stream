package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func Intent(ctx context.Context, reader store.Reader, tenantID, intentID string) (domain.IntentView, error) {
	intent, err := reader.Intent(ctx, tenantID, intentID)
	if err != nil {
		return domain.IntentView{}, fmt.Errorf("intent %s: %w", intentID, err)
	}
	return intent, nil
}

func DecisionIntents(ctx context.Context, reader store.Reader, tenantID, decisionID string) ([]domain.IntentView, error) {
	intents, err := reader.DecisionIntents(ctx, tenantID, decisionID)
	if err != nil {
		return nil, fmt.Errorf("intents of decision %s: %w", decisionID, err)
	}
	return intents, nil
}
