package episodes

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// DecisionView is one Decision a worker returned for an episode and how the
// runtime validated it.
type DecisionView = domain.DecisionView

// Decisions reads every Decision recorded for one episode, in order. It only
// reads.
func Decisions(ctx context.Context, db *storage.DB, episodeID string) ([]DecisionView, error) {
	return app.Decisions(ctx, store.New(db), episodeID)
}
