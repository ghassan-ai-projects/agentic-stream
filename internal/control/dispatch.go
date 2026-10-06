package control

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewDispatchAuthorization binds a read-only final readiness gate to one
// command's tenant and target. Adapters invoke this lower-level capability;
// they never call back into the dispatcher or another upstream service.
func NewDispatchAuthorization(db *storage.DB, reader interlock.Reader, tenantID, target string) actionport.Authorization {
	persistence := store.New(db)
	return actionport.Authorization{Check: func(ctx context.Context) error {
		return app.AuthorizeDispatch(ctx, persistence, reader, tenantID, target)
	}}
}
