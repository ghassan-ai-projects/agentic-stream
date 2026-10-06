package eventlog

import (
	"context"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EntityWindow is a read-only evidence window over one tenant's entity.
type EntityWindow = domain.EntityWindow

// EntityEvent is one event of an entity evidence window.
type EntityEvent = domain.EntityEvent

// ReadEntityWindow visits the window's events in event-time then log order,
// at most MaxRows of them, until visit reports it wants no more.
func ReadEntityWindow(ctx context.Context, db *storage.DB, window EntityWindow, visit func(EntityEvent) (bool, error)) error {
	return app.New(nil, store.New(db)).ReadEntityEvents(ctx, window, visit)
}
