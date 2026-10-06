package domain

import (
	"fmt"
	"time"
)

// SchedulerItem is one durable scheduler entry.
type SchedulerItem struct {
	SchedulerItemID  string     // unique scheduler item identity.
	Kind             string     // standard or reconsider.
	TriggerID        string     // trigger evaluation that admitted this item.
	SituationID      string     // situation being reasoned about.
	SituationVersion int        // immutable situation version bound to this item.
	Lane             string     // fast or deep lane.
	Priority         float64    // admission score used for ordering.
	Status           string     // pending, admitted, coalesced, expired, canceled, completed.
	NotBefore        *time.Time // earliest time the item may be picked.
	ExpiresAt        time.Time  // latest time the item remains useful.
}

// CheckStillPending fails when a pending-only transition changed no item.
func CheckStillPending(rows int64, schedulerItemID string) error {
	if rows != 1 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}

// Operation names for the two ways a pending scheduler item leaves the queue
// unadmitted; they label errors.
const (
	OperationSkipCostRejected = "skip cost-rejected scheduler item"
	OperationCoalesceSkipped  = "coalesce scheduler item"
)
