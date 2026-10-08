package store

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

// InterlockState is the durable global interlock.
type InterlockState = interlock.State

// ReadInterlock returns the interlock inside the transaction.
func (t *Tx) ReadInterlock(ctx context.Context) (InterlockState, error) {
	return interlock.Read(ctx, t.tx) //nolint:wrapcheck // The interlock module names the failed step.
}

// TripInterlock blocks the action plane inside the transaction.
func (t *Tx) TripInterlock(ctx context.Context, reason, now string) (InterlockState, error) {
	return interlock.Trip(ctx, t.tx, reason, now) //nolint:wrapcheck // The interlock module names the failed step.
}

// ClearInterlock reopens the action plane inside the transaction.
func (t *Tx) ClearInterlock(ctx context.Context, reason, now string) (InterlockState, error) {
	return interlock.Clear(ctx, t.tx, reason, now) //nolint:wrapcheck // The interlock module names the failed step.
}
