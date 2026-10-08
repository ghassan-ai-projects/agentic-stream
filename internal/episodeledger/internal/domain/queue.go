package domain

import (
	"cmp"
	"slices"
	"time"
)

// QueuedItem is a pending scheduler item with the times that decide when it
// may be admitted.
type QueuedItem struct {
	SchedulerItemID string
	CreatedAt       time.Time
	NotBefore       *time.Time
	ExpiresAt       time.Time
}

// DueItem is a pending scheduler item whose admission window is open: the
// earliest instant it may be admitted and the instant it stops being useful.
type DueItem struct {
	SchedulerItemID string
	AdmitAt         time.Time
	ExpiresAt       time.Time
}

// DueItems selects the queued items that may be admitted by now, in queue
// order: not-before (none first), then creation, then identity. An item is due
// when its admission time, the later of creation and not-before, has arrived
// and still precedes its expiry.
func DueItems(queued []QueuedItem, now time.Time) []DueItem {
	ordered := slices.SortedFunc(slices.Values(queued), compareQueueOrder)
	var due []DueItem
	for _, item := range ordered {
		admitAt := item.admitAt()
		if item.ExpiresAt.After(admitAt) && !admitAt.After(now) {
			due = append(due, DueItem{SchedulerItemID: item.SchedulerItemID, AdmitAt: admitAt, ExpiresAt: item.ExpiresAt})
		}
	}
	return due
}

// Stale reports whether the item has reached its expiry by now.
func (d DueItem) Stale(now time.Time) bool { return !d.ExpiresAt.After(now) }

func (q QueuedItem) admitAt() time.Time {
	if q.NotBefore != nil && q.NotBefore.After(q.CreatedAt) {
		return *q.NotBefore
	}
	return q.CreatedAt
}

func compareQueueOrder(a, b QueuedItem) int {
	return cmp.Or(
		compareNotBefore(a.NotBefore, b.NotBefore),
		a.CreatedAt.Compare(b.CreatedAt),
		cmp.Compare(a.SchedulerItemID, b.SchedulerItemID),
	)
}

func compareNotBefore(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(*b)
}
