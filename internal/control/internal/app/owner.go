package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// Owner is the configuration of one runtime owner. A nil Owner is unconfigured
// and every operation refuses it.
type Owner struct {
	Store    store.Store
	Instance string
	Lease    time.Duration
	Now      func() time.Time
}

// Claim acquires or renews the singleton lease for epoch. A valid lease held
// by another epoch is rejected transactionally.
func Claim(ctx context.Context, o *Owner, epoch string) error {
	return ClaimAndRecover(ctx, o, epoch, nil)
}

// ClaimAndRecover acquires the singleton lease and runs recover in the same
// transaction. If recovery fails, ownership is not committed.
func ClaimAndRecover(ctx context.Context, o *Owner, epoch string, recover store.Recovery) error {
	if o == nil || !o.Store.Configured() || epoch == "" || o.Instance == "" {
		return domain.ErrOwnerNotConfigured
	}
	now := o.Now()
	return o.Store.WithTx(ctx, func(tx *store.Tx) error {
		return claimAndRecover(ctx, o, tx, epoch, now, recover)
	})
}

func claimAndRecover(ctx context.Context, o *Owner, tx *store.Tx, epoch string, now time.Time, recover store.Recovery) error {
	if err := claim(ctx, o, tx, epoch, now); err != nil {
		return err
	}
	if recover != nil {
		if err := tx.Recover(recover, now); err != nil {
			return err
		}
	}
	return assert(ctx, o, tx, epoch)
}

func claim(ctx context.Context, o *Owner, tx *store.Tx, epoch string, now time.Time) error {
	until := now.Add(sources.OrLease(o.Lease))
	if err := tx.ClaimLease(ctx, epoch, o.Instance, domain.TimeText(now), domain.TimeText(until)); err != nil {
		return err
	}
	recordedEpoch, recordedInstance, err := tx.RecordedOwner(ctx)
	if err != nil {
		return err
	}
	return domain.CheckHolder(domain.Holder{Epoch: recordedEpoch, Instance: recordedInstance}, epoch, o.Instance)
}

// Renew extends the lease only when this epoch still owns it and has not
// already expired. A zero-row update means ownership was lost.
func Renew(ctx context.Context, o *Owner, epoch string) error {
	if o == nil || !o.Store.Configured() || epoch == "" {
		return domain.ErrOwnerNotConfigured
	}
	now := o.Now()
	until := now.Add(sources.OrLease(o.Lease))
	rows, err := o.Store.Autocommit().RenewLease(ctx, epoch, o.Instance, domain.TimeText(now), domain.TimeText(until))
	if err != nil {
		return err
	}
	return domain.CheckOwnerMutation(rows)
}

// Release clears the singleton lease only for the owning epoch.
func Release(ctx context.Context, o *Owner, epoch string) error {
	if o == nil || !o.Store.Configured() || epoch == "" {
		return domain.ErrOwnerNotConfigured
	}
	rows, err := o.Store.Autocommit().ReleaseLease(ctx, epoch, o.Instance, domain.TimeText(o.Now()))
	if err != nil {
		return err
	}
	return domain.CheckOwnerMutation(rows)
}

// Assert verifies that epoch currently owns an unexpired lease inside tx.
// Callers use this as the write-fencing predicate for runtime-owned state.
func Assert(ctx context.Context, o *Owner, tx *store.Tx, epoch string) error {
	if o == nil || !tx.Open() || epoch == "" {
		return domain.ErrOwnerNotConfigured
	}
	return assert(ctx, o, tx, epoch)
}

func assert(ctx context.Context, o *Owner, tx *store.Tx, epoch string) error {
	held, err := tx.HoldsLease(ctx, epoch, o.Instance, domain.TimeText(o.Now()))
	if err != nil {
		return err
	}
	if !held {
		return domain.ErrRuntimeOwnerBusy
	}
	return nil
}
