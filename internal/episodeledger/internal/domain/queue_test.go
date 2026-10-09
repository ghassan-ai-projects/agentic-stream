package domain

import (
	"slices"
	"testing"
	"time"
)

func TestDueItemsAppliesWindowRulesInQueueOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time {
		moment := base.Add(time.Duration(minutes) * time.Minute)
		return &moment
	}
	queued := []QueuedItem{
		{SchedulerItemID: "b-plain", CreatedAt: *at(2), ExpiresAt: *at(60)},
		{SchedulerItemID: "a-plain", CreatedAt: *at(2), ExpiresAt: *at(60)},
		{SchedulerItemID: "z-early", CreatedAt: *at(1), ExpiresAt: *at(60)},
		{SchedulerItemID: "debounced", CreatedAt: *at(0), NotBefore: at(5), ExpiresAt: *at(60)},
		{SchedulerItemID: "debounced-earlier", CreatedAt: *at(3), NotBefore: at(4), ExpiresAt: *at(60)},
		{SchedulerItemID: "created-in-future", CreatedAt: *at(30), ExpiresAt: *at(60)},
		{SchedulerItemID: "not-before-future", CreatedAt: *at(0), NotBefore: at(30), ExpiresAt: *at(60)},
		{SchedulerItemID: "expires-at-admission", CreatedAt: *at(0), NotBefore: at(6), ExpiresAt: *at(6)},
		{SchedulerItemID: "expired-before-not-before", CreatedAt: *at(0), NotBefore: at(7), ExpiresAt: *at(3)},
	}
	var ids []string
	for _, item := range DueItems(queued, *at(10)) {
		ids = append(ids, item.SchedulerItemID)
	}
	want := []string{"z-early", "a-plain", "b-plain", "debounced-earlier", "debounced"}
	if !slices.Equal(ids, want) {
		t.Fatalf("due order = %v, want %v", ids, want)
	}
}

func TestDueItemAdmitAtIsTheLaterOfCreationAndNotBefore(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := created.Add(time.Hour)
	for name, test := range map[string]struct {
		notBefore *time.Time
		want      time.Time
	}{
		"none":            {nil, created},
		"later":           {&later, later},
		"before-creation": {&created, created},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			due := DueItems([]QueuedItem{{SchedulerItemID: "i", CreatedAt: created, NotBefore: test.notBefore, ExpiresAt: later.Add(time.Hour)}}, later)
			if len(due) != 1 || !due[0].AdmitAt.Equal(test.want) {
				t.Fatalf("due = %+v, want admit at %v", due, test.want)
			}
		})
	}
}

func TestDueItemIsStaleFromItsExpiryInstant(t *testing.T) {
	t.Parallel()
	expiry := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	item := DueItem{ExpiresAt: expiry}
	for name, test := range map[string]struct {
		now  time.Time
		want bool
	}{
		"before": {expiry.Add(-time.Nanosecond), false},
		"at":     {expiry, true},
		"after":  {expiry.Add(time.Nanosecond), true},
	} {
		if got := item.Stale(test.now); got != test.want {
			t.Errorf("%s: Stale = %v, want %v", name, got, test.want)
		}
	}
}

func TestPollSeparatesTheAdmittableItemFromThoseThatCanNeverRun(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	debounced := at(5)
	queue := PendingQueue{
		Items: []QueuedItem{
			{SchedulerItemID: "b-stale", CreatedAt: at(0), ExpiresAt: at(10)},
			{SchedulerItemID: "a-fresh", CreatedAt: at(1), ExpiresAt: at(60)},
			{SchedulerItemID: "c-later", CreatedAt: at(30), ExpiresAt: at(60)},
			{SchedulerItemID: "d-dead-window", CreatedAt: at(0), NotBefore: &debounced, ExpiresAt: at(5)},
			{SchedulerItemID: "e-dead-window", CreatedAt: at(0), NotBefore: &debounced, ExpiresAt: at(5)},
		},
		Unreadable: []UnreadableItem{{SchedulerItemID: "z-bad", Column: "expires_at"}, {SchedulerItemID: "y-bad", Column: "created_at"}},
	}
	poll := Poll(queue, at(12))
	want := []ExpiredItem{
		{SchedulerItemID: "y-bad", Reason: "unreadable created_at"},
		{SchedulerItemID: "z-bad", Reason: "unreadable expires_at"},
		{SchedulerItemID: "b-stale", Reason: ReasonExpired},
		{SchedulerItemID: "d-dead-window", Reason: ReasonExpired},
		{SchedulerItemID: "e-dead-window", Reason: ReasonExpired},
	}
	if poll.Next != "a-fresh" || !poll.Found || !slices.Equal(poll.Expired, want) {
		t.Fatalf("poll = %+v, want next a-fresh and expired %v", poll, want)
	}
}

func TestPollOfAnEmptyOrFutureQueueFindsNothing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := QueuedItem{SchedulerItemID: "future", CreatedAt: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour)}
	for name, queue := range map[string]PendingQueue{"empty": {}, "future": {Items: []QueuedItem{future}}} {
		if poll := Poll(queue, now); poll.Found || poll.Next != "" || len(poll.Expired) != 0 {
			t.Errorf("%s: poll = %+v", name, poll)
		}
	}
}

func TestPollExpiresAnItemAtItsExpiryInstantAndNotOneNanosecondBefore(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiry := created.Add(time.Hour)
	queue := PendingQueue{Items: []QueuedItem{{SchedulerItemID: "item", CreatedAt: created, ExpiresAt: expiry}}}
	if poll := Poll(queue, expiry.Add(-time.Nanosecond)); poll.Next != "item" || len(poll.Expired) != 0 {
		t.Fatalf("one nanosecond before expiry: %+v", poll)
	}
	if poll := Poll(queue, expiry); poll.Found || len(poll.Expired) != 1 {
		t.Fatalf("at expiry: %+v", poll)
	}
}

func TestAPendingOnlyTransitionMustChangeExactlyOneItem(t *testing.T) {
	t.Parallel()
	if err := CheckStillPending(1, "item"); err != nil {
		t.Fatalf("one changed row refused: %v", err)
	}
	for _, rows := range []int64{0, 2} {
		err := CheckStillPending(rows, "item")
		if err == nil || err.Error() != "scheduler item item is no longer pending" {
			t.Errorf("rows=%d: err = %v, want the item named as no longer pending", rows, err)
		}
	}
}
