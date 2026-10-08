package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// Decisions reads every Decision recorded for one episode, in order.
func Decisions(ctx context.Context, s store.Store, episodeID string) ([]domain.DecisionView, error) {
	return s.Decisions(ctx, episodeID) //nolint:wrapcheck // The store names the failed read.
}
