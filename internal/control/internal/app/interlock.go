package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// ReadInterlock returns the global interlock.
func ReadInterlock(ctx context.Context, s store.Store) (store.InterlockState, error) {
	var state store.InterlockState
	err := s.WithTx(ctx, func(tx *store.Tx) (err error) {
		state, err = tx.ReadInterlock(ctx)
		return err
	})
	return state, err
}

// TripInterlock blocks every effect for reason. It needs no runtime ownership:
// tripping only stops effects, and an emergency stop must work when the
// runtime is hung.
func TripInterlock(ctx context.Context, s store.Store, reason string, now time.Time) (store.InterlockState, error) {
	var state store.InterlockState
	err := s.WithTx(ctx, func(tx *store.Tx) (err error) {
		state, err = tx.TripInterlock(ctx, reason, domain.TimeText(now))
		return err
	})
	return state, err
}

// ClearInterlock reopens the action plane for reason inside one transaction
// that first asserts the owner's lease, so it cannot race a dispatch.
func ClearInterlock(ctx context.Context, o *Owner, epoch, reason string) (store.InterlockState, error) {
	if o == nil || !o.Store.Configured() || epoch == "" {
		return store.InterlockState{}, domain.ErrOwnerNotConfigured
	}
	var state store.InterlockState
	err := o.Store.WithTx(ctx, func(tx *store.Tx) error {
		if err := assert(ctx, o, tx, epoch); err != nil {
			return err
		}
		var err error
		state, err = tx.ClearInterlock(ctx, reason, domain.TimeText(o.Now()))
		return err
	})
	return state, err
}
