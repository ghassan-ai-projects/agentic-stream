package domain

import (
	"errors"
	"time"
)

// ErrRuntimeOwnerBusy means another runtime currently owns the database lease.
var ErrRuntimeOwnerBusy = errors.New("runtime owner lease is held by another process")

// DefaultLease is the lease duration used when none is configured.
const DefaultLease = time.Minute

// LeaseDuration returns the configured lease, or the default.
func LeaseDuration(lease time.Duration) time.Duration {
	if lease > 0 {
		return lease
	}
	return DefaultLease
}

// Holder identifies the owner recorded in the lease row.
type Holder struct {
	Epoch    string
	Instance string
}

// CheckHolder refuses a claim whose recorded owner is another epoch or
// another instance.
func CheckHolder(recorded Holder, epoch, instance string) error {
	if recorded.Epoch != epoch || recorded.Instance != instance {
		return ErrRuntimeOwnerBusy
	}
	return nil
}

// CheckOwnerMutation requires a renew or release to have changed exactly the
// one lease row; anything else means ownership was lost.
func CheckOwnerMutation(rows int64) error {
	if rows != 1 {
		return ErrRuntimeOwnerBusy
	}
	return nil
}

// ErrOwnerNotConfigured means the runtime owner is missing a required part.
var ErrOwnerNotConfigured = errors.New("runtime owner is not configured")

// ErrEpochControlNotConfigured means epoch control is missing a required part.
var ErrEpochControlNotConfigured = errors.New("epoch control is not configured")

// TimeText encodes a time for the owner and epoch tables. They are compared as
// SQLite TEXT, so fixed-width nanoseconds keep chronological order even when a
// time has no fractional component.
func TimeText(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

// ErrDispatchGateNotConfigured means the dispatch readiness gate lacks its
// database or interlock reader.
var ErrDispatchGateNotConfigured = errors.New("dispatch readiness gate is not configured")
