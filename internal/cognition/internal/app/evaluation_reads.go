package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func TriggerEvaluation(ctx context.Context, reader store.Reader, tenantID, triggerID string) (domain.TriggerEvaluationRecord, error) {
	record, err := reader.TriggerEvaluation(ctx, tenantID, triggerID)
	if err != nil {
		return domain.TriggerEvaluationRecord{}, fmt.Errorf("trigger evaluation %s: %w", triggerID, err)
	}
	return record, nil
}

func TriggerEvaluations(ctx context.Context, reader store.Reader, tenantID, situationID string, version int) ([]domain.TriggerEvaluationRecord, error) {
	records, err := reader.TriggerEvaluations(ctx, tenantID, situationID, version)
	if err != nil {
		return nil, fmt.Errorf("trigger evaluations of %s version %d: %w", situationID, version, err)
	}
	return records, nil
}
