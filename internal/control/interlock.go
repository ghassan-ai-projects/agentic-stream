package control

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// InterlockState is the durable global interlock: status, reason, time and
// version.
type InterlockState = store.InterlockState

// ReadInterlock returns the global action-plane interlock.
func ReadInterlock(ctx context.Context, db *storage.DB) (InterlockState, error) {
	return app.ReadInterlock(ctx, store.New(db)) //nolint:wrapcheck // The interlock module names the failed step.
}

// TripInterlock blocks every effect for reason, without runtime ownership, so
// the software emergency stop works while the runtime runs or is hung.
func TripInterlock(ctx context.Context, db *storage.DB, reason string) (InterlockState, error) {
	return app.TripInterlock(ctx, store.New(db), reason, time.Now().UTC()) //nolint:wrapcheck // The interlock module names the failed step.
}

// ClearInterlock reopens the action plane for reason. epoch must hold the
// runtime owner lease; the clear is fenced by it in the same transaction.
func (o *RuntimeOwner) ClearInterlock(ctx context.Context, epoch, reason string) (InterlockState, error) {
	return app.ClearInterlock(ctx, o.session(), epoch, reason) //nolint:wrapcheck // The interlock module names the failed step.
}
