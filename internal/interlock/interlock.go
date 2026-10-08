package interlock

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/store"
)

// ErrTripped means the action plane is globally blocked by a durable interlock.
var ErrTripped = domain.ErrTripped

// Assert reads the singleton interlock inside the caller's transaction and
// fails closed, wrapping ErrTripped, when it is absent or not ready. Policy
// calls it before creating a command and again before the effect is delivered.
func Assert(ctx context.Context, tx *sql.Tx) error {
	if err := store.Assert(ctx, tx); err != nil {
		return fmt.Errorf("assert interlock: %w", err)
	}
	return nil
}

// State is the durable global interlock: its status, why, when, and version.
type State = domain.State

// Transactor opens one database transaction; *storage.DB satisfies it.
type Transactor interface {
	WithTx(ctx context.Context, fn func(*sql.Tx) error) error
}

// Fence is checked inside the transaction of a change, before it writes.
type Fence func(ctx context.Context, tx *sql.Tx) error

// Status returns the interlock.
func Status(ctx context.Context, db Transactor) (State, error) {
	var state State
	err := db.WithTx(ctx, func(tx *sql.Tx) (err error) {
		state, err = store.Read(ctx, tx)
		return err
	})
	if err != nil {
		return State{}, fmt.Errorf("read interlock: %w", err)
	}
	return state, nil
}

// Trip blocks every effect for reason. Tripping only stops effects, so it
// takes no fence: an emergency stop must work when the runtime is hung.
func Trip(ctx context.Context, db Transactor, reason string, now time.Time) (State, error) {
	return within(ctx, db, nil, func(tx *sql.Tx) (State, error) { return TripIn(ctx, tx, reason, now) })
}

// Clear reopens the action plane for reason. The fence runs in the same
// transaction before the write, so a caller that does not hold it cannot
// reopen the plane while a dispatch races.
func Clear(ctx context.Context, db Transactor, fence Fence, reason string, now time.Time) (State, error) {
	if fence == nil {
		return State{}, fmt.Errorf("clear interlock: a fence is required")
	}
	return within(ctx, db, fence, func(tx *sql.Tx) (State, error) { return ClearIn(ctx, tx, reason, now) })
}

// TripIn blocks every effect inside the caller's transaction.
func TripIn(ctx context.Context, tx *sql.Tx, reason string, now time.Time) (State, error) {
	state, err := store.Change(ctx, tx, domain.StatusTripped, reason, now.UTC().Format(timeLayout))
	if err != nil {
		return State{}, fmt.Errorf("trip interlock: %w", err)
	}
	return state, nil
}

// ClearIn reopens the action plane inside the caller's transaction; the caller
// owns the fence.
func ClearIn(ctx context.Context, tx *sql.Tx, reason string, now time.Time) (State, error) {
	state, err := store.Change(ctx, tx, domain.StatusReady, reason, now.UTC().Format(timeLayout))
	if err != nil {
		return State{}, fmt.Errorf("clear interlock: %w", err)
	}
	return state, nil
}

func within(ctx context.Context, db Transactor, fence Fence, apply func(*sql.Tx) (State, error)) (State, error) {
	var state State
	err := db.WithTx(ctx, func(tx *sql.Tx) (err error) {
		if fence != nil {
			if err = fence(ctx, tx); err != nil {
				return err
			}
		}
		state, err = apply(tx)
		return err
	})
	if err != nil {
		return State{}, fmt.Errorf("interlock transaction: %w", err)
	}
	return state, nil
}

const timeLayout = "2006-01-02T15:04:05.000000000Z"
