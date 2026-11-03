package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

func (s *Service) SituationFeatures(ctx context.Context, situationID string) (map[string]any, bool, error) {
	state, found, err := s.store.CurrentState(ctx, situationID)
	if err != nil {
		return nil, false, fmt.Errorf("features of situation %s: %w", situationID, err)
	}
	if !found {
		return nil, false, nil
	}
	features, err := domain.SituationFeatures(s.spec, situationID, state)
	if err != nil {
		return nil, false, fmt.Errorf("features of situation %s: %w", situationID, err)
	}
	return features, true, nil
}
