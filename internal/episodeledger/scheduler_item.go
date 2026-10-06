package episodeledger

import "time"

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
