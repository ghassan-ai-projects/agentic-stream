package control

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewDispatchAuthorization returns the read-only final readiness gate. Adapters invoke this lower-level capability;
// they never call back into the dispatcher or another upstream service.
func NewDispatchAuthorization(db *storage.DB) actionport.Authorization {
	persistence := store.New(db)
	return actionport.Authorization{Check: func(ctx context.Context) error {
		return app.AuthorizeDispatch(ctx, persistence)
	}}
}
