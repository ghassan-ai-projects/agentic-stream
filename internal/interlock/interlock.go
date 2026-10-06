package interlock

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/store"
)

// ErrTripped means the action plane is globally blocked by a durable interlock.
var ErrTripped = domain.ErrTripped

// Reader is the narrow, read-only surface used immediately before command
// creation and again immediately before effect delivery.
type Reader interface {
	Assert(context.Context, *sql.Tx, string, string, string) error
}

// DurableReader reads the singleton interlock inside the caller's transaction
// and fails closed when it is absent or not ready.
type DurableReader = store.DurableReader

// Set changes the durable interlock state. Callers must separately fence this
// mutation with the active runtime owner.
func Set(ctx context.Context, tx *sql.Tx, status, reason string, version int64, now string) error {
	return store.Set(ctx, tx, status, reason, version, now) //nolint:wrapcheck // The store names the failed step.
}
