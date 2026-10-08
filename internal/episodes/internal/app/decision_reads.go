package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

func Decisions(ctx context.Context, s store.Store, episodeID string) ([]domain.DecisionView, error) {
	decisions, err := s.Decisions(ctx, episodeID)
	if err != nil {
		return nil, fmt.Errorf("decisions of episode %s: %w", episodeID, err)
	}
	return decisions, nil
}
