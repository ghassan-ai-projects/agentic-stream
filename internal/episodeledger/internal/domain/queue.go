package domain

import (
	"cmp"
	"slices"
	"time"
)

type QueuedItem struct {
	SchedulerItemID string
	CreatedAt       time.Time
	NotBefore       *time.Time
	ExpiresAt       time.Time
}

type DueItem struct {
	SchedulerItemID string
	AdmitAt         time.Time
	ExpiresAt       time.Time
}

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

func (d DueItem) Stale(now time.Time) bool { return !d.ExpiresAt.After(now) }

const ReasonExpired = "expired before admission"

type UnreadableItem struct {
	SchedulerItemID string
	Column          string
}

type PendingQueue struct {
	Items      []QueuedItem
	Unreadable []UnreadableItem
}

type ExpiredItem struct {
	SchedulerItemID string
	Reason          string
}

type QueuePoll struct {
	Next    string
	Found   bool
	Expired []ExpiredItem
}

func Poll(queue PendingQueue, now time.Time) QueuePoll {
	poll := QueuePoll{Expired: unreadableExpiries(queue.Unreadable)}
	poll.Expired = append(poll.Expired, staleExpiries(queue.Items, now)...)
	poll.Next, poll.Found = firstAdmittable(DueItems(queue.Items, now), now)
	return poll
}

func unreadableExpiries(unreadable []UnreadableItem) []ExpiredItem {
	var expired []ExpiredItem
	for _, item := range slices.SortedFunc(slices.Values(unreadable), compareUnreadable) {
		expired = append(expired, ExpiredItem{SchedulerItemID: item.SchedulerItemID, Reason: "unreadable " + item.Column})
	}
	return expired
}

func compareUnreadable(a, b UnreadableItem) int {
	return cmp.Compare(a.SchedulerItemID, b.SchedulerItemID)
}

func staleExpiries(queued []QueuedItem, now time.Time) []ExpiredItem {
	var expired []ExpiredItem
	for _, item := range slices.SortedFunc(slices.Values(queued), compareQueueOrder) {
		if !item.ExpiresAt.After(now) {
			expired = append(expired, ExpiredItem{SchedulerItemID: item.SchedulerItemID, Reason: ReasonExpired})
		}
	}
	return expired
}

func firstAdmittable(due []DueItem, now time.Time) (string, bool) {
	for _, item := range due {
		if !item.Stale(now) {
			return item.SchedulerItemID, true
		}
	}
	return "", false
}

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

func (q *PendingQueue) Add(item QueuedItem, unreadableColumn string) {
	if unreadableColumn != "" {
		q.Unreadable = append(q.Unreadable, UnreadableItem{SchedulerItemID: item.SchedulerItemID, Column: unreadableColumn})
		return
	}
	q.Items = append(q.Items, item)
}
