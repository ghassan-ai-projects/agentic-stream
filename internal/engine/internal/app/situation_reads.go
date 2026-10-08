package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

func ListSituations(ctx context.Context, reader store.Reader, tenantID, entityID string) ([]domain.SituationSummary, error) {
	situations, err := reader.ListSituations(ctx, tenantID, entityID)
	if err != nil {
		return nil, fmt.Errorf("situations of tenant %s: %w", tenantID, err)
	}
	return situations, nil
}

func SituationVersion(ctx context.Context, reader store.Reader, tenantID, situationID string, version int) (domain.SituationVersionRecord, error) {
	record, err := reader.SituationVersion(ctx, tenantID, situationID, version)
	if err != nil {
		return domain.SituationVersionRecord{}, fmt.Errorf("situation %s version %d: %w", situationID, version, err)
	}
	return record, nil
}
