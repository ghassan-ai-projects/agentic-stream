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

// State is the durable global interlock: its status, why, when, and version.
type State = domain.State

// Read returns the interlock inside the caller's transaction.
func Read(ctx context.Context, tx *sql.Tx) (State, error) {
	return store.Read(ctx, tx) //nolint:wrapcheck // The store names the failed step.
}

// Trip blocks the action plane for reason. Tripping only stops effects, so it
// needs no runtime ownership: an emergency stop must work when the runtime is
// hung.
func Trip(ctx context.Context, tx *sql.Tx, reason, now string) (State, error) {
	return store.Change(ctx, tx, domain.StatusTripped, reason, now) //nolint:wrapcheck // The store names the failed step.
}

// Clear reopens the action plane for reason. Callers must fence it with the
// runtime owner inside the same transaction, so it cannot race a dispatch.
func Clear(ctx context.Context, tx *sql.Tx, reason, now string) (State, error) {
	return store.Change(ctx, tx, domain.StatusReady, reason, now) //nolint:wrapcheck // The store names the failed step.
}
