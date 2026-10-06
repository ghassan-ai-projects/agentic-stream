package control

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
)

// Refusals the control plane reports.
var (
	// ErrRuntimeOwnerBusy means another runtime currently owns the database lease.
	ErrRuntimeOwnerBusy = domain.ErrRuntimeOwnerBusy
	// ErrEpochKilled means the epoch was killed and every later decision under it is refused.
	ErrEpochKilled = domain.ErrEpochKilled
	// ErrEpochUnbound means a decision has no recorded owner epoch.
	ErrEpochUnbound = domain.ErrEpochUnbound
	// ErrEpochDraining means the epoch is draining and new episodes are refused.
	ErrEpochDraining = domain.ErrEpochDraining
	// ErrCostReservationRejected means an aggregate cost limit or kill switch
	// denied a new episode reservation; it is an expected admission outcome.
	ErrCostReservationRejected = domain.ErrCostReservationRejected
)

// CostCeilings is an operator's cost configuration. A nil field leaves the
// current value in place; the kill switch also applies to the tenant ceiling.
type CostCeilings = domain.CostCeilings

// utcNow returns the configured clock in UTC, or the physical clock.
func utcNow(now func() time.Time) func() time.Time {
	if now == nil {
		return func() time.Time { return time.Now().UTC() }
	}
	return func() time.Time { return now().UTC() }
}
