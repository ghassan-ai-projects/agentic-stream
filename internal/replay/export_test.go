package replay

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/replaytest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func WithSeededDatabases(ctx context.Context) context.Context {
	return replaytest.WithDatabaseOpener(ctx, storagetest.Open)
}
