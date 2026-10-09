package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

type queued struct {
	id        string
	situation string
	created   int
	notBefore int
	expires   int
}

func minutes(n int) time.Time { return now.Add(time.Duration(n) * time.Minute) }

func enqueue(t *testing.T, ctx context.Context, tx *store.Tx, seeds ...queued) {
	t.Helper()
	for _, seed := range seeds {
		situation := cmp.Or(seed.situation, "s")
		item := domain.SchedulerItem{SchedulerItemID: seed.id, TriggerID: "trigger-" + seed.id, SituationID: situation, SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: minutes(seed.expires)}
		if seed.notBefore != 0 {
			notBefore := minutes(seed.notBefore)
			item.NotBefore = &notBefore
		}
		key := sha256.Sum256([]byte(seed.id))
		must(t, UpsertSchedulerItem(ctx, tx, item, "tenant", key[:], minutes(seed.created)))
	}
}

func recordAdmittedTrigger(t *testing.T, ctx context.Context, raw *sql.Tx, item queued) {
	t.Helper()
	execSQL(t, ctx, raw, `INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at)
		VALUES (?, 'tenant', 'deployment', 'alarm', ?, 1, 1, 0, 'fast', 'admitted', X'5B5D', ?, 'now')`, "trigger-"+item.id, cmp.Or(item.situation, "s"), make([]byte, 32))
}

func itemStatus(t *testing.T, ctx context.Context, raw *sql.Tx, id string) string {
	t.Helper()
	return queryText(t, ctx, raw, "SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", id)
}

func dueIDs(t *testing.T, ctx context.Context, tx *store.Tx, at time.Time) []string {
	t.Helper()
	due, err := DueSchedulerItems(ctx, tx, "tenant", at)
	must(t, err)
	var ids []string
	for _, item := range due {
		ids = append(ids, item.SchedulerItemID)
	}
	return ids
}

func expiredIDs(poll domain.QueuePoll) []string {
	var ids []string
	for _, item := range poll.Expired {
		ids = append(ids, item.SchedulerItemID)
	}
	return ids
}

func TestAnItemIdClashKeepsTheExistingItemAndATriggerKeepsItsFirstId(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		enqueue(t, ctx, tx, queued{id: "item", expires: 60})
		clash := domain.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger-other", SituationID: "s", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: minutes(60)}
		must(t, UpsertSchedulerItem(ctx, tx, clash, "tenant", append([]byte{1}, make([]byte, 31)...), now))
		reissued := domain.SchedulerItem{SchedulerItemID: "item-new-id", TriggerID: "trigger-item", SituationID: "s", SituationVersion: 2, Kind: "standard", Lane: "deep", Priority: 9, Status: "pending", ExpiresAt: minutes(60)}
		must(t, UpsertSchedulerItem(ctx, tx, reissued, "tenant", append([]byte{2}, make([]byte, 31)...), now))
		row := queryText(t, ctx, raw, "SELECT COUNT(*) || '|' || MIN(scheduler_item_id) || '|' || MIN(trigger_id) || '|' || MIN(situation_version) || '|' || MIN(lane) || '|' || MIN(priority) FROM scheduler_items")
		if want := "1|item|trigger-item|2|deep|9.0"; row != want {
			t.Fatalf("queue = %s, want %s: one item that kept its identity and took the new priority", row, want)
		}
	})
}

func TestAPendingItemLeavesTheQueueOnlyOnceAndNoOtherTransitionRevivesIt(t *testing.T) {
	t.Parallel()
	type operation struct {
		name  string
		apply func(context.Context, *store.Tx, string) error
		state string
	}
	operations := []operation{
		{"admit", func(ctx context.Context, tx *store.Tx, id string) error {
			return MarkSchedulerItemAdmitted(ctx, tx, id, now)
		}, "admitted"},
		{"skip", func(ctx context.Context, tx *store.Tx, id string) error { return CoalesceSkippedItem(ctx, tx, id, now) }, "coalesced"},
		{"reject cost", func(ctx context.Context, tx *store.Tx, id string) error {
			return CoalesceCostRejectedItem(ctx, tx, id, now)
		}, "coalesced"},
		{"expire", func(ctx context.Context, tx *store.Tx, id string) error { return ExpireSchedulerItem(ctx, tx, id, now) }, "expired"},
	}
	for _, first := range operations {
		t.Run(first.name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				enqueue(t, ctx, tx, queued{id: "item", expires: 60})
				must(t, first.apply(ctx, tx, "item"))
				for _, again := range operations {
					err := again.apply(ctx, tx, "item")
					if err == nil || err.Error() != "scheduler item item is no longer pending" {
						t.Errorf("%s after %s = %v, want the item refused as no longer pending", again.name, first.name, err)
					}
				}
				if status := itemStatus(t, ctx, raw, "item"); status != first.state {
					t.Fatalf("status = %s, want %s", status, first.state)
				}
			})
		})
	}
}

func TestAnUnknownItemCannotLeaveThePendingQueue(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		for name, err := range map[string]error{
			"admit":  MarkSchedulerItemAdmitted(ctx, tx, "missing", now),
			"skip":   CoalesceSkippedItem(ctx, tx, "missing", now),
			"cost":   CoalesceCostRejectedItem(ctx, tx, "missing", now),
			"expire": ExpireSchedulerItem(ctx, tx, "missing", now),
		} {
			if err == nil || err.Error() != "scheduler item missing is no longer pending" {
				t.Errorf("%s of an unknown item = %v", name, err)
			}
		}
	})
}

func TestExpiredItemsLeaveThePendingQueueAndAreNotPolledAgain(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		enqueue(t, ctx, tx, queued{id: "a-stale", expires: 12}, queued{id: "b-fresh", expires: 60}, queued{id: "c-stale", created: 1, expires: 13})
		at := minutes(15)
		poll, err := PollSchedulerQueue(ctx, tx, "tenant", at)
		if err != nil || poll.Next != "b-fresh" || !slices.Equal(expiredIDs(poll), []string{"a-stale", "c-stale"}) || poll.Expired[0].Reason != domain.ReasonExpired {
			t.Fatalf("poll = %+v err=%v", poll, err)
		}
		for _, expired := range poll.Expired {
			must(t, ExpireSchedulerItem(ctx, tx, expired.SchedulerItemID, at))
		}
		if got := itemStatus(t, ctx, raw, "a-stale") + "|" + itemStatus(t, ctx, raw, "b-fresh"); got != "expired|pending" {
			t.Fatalf("statuses = %s, want the stale item expired and the fresh one pending", got)
		}
		again, err := PollSchedulerQueue(ctx, tx, "tenant", at)
		if err != nil || again.Next != "b-fresh" || len(again.Expired) != 0 {
			t.Fatalf("second poll = %+v err=%v, want no rescan of expired items", again, err)
		}
	})
}

func TestAnItemExpiresExactlyAtItsExpiryInstant(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		enqueue(t, ctx, tx, queued{id: "item", expires: 10})
		expiry := minutes(10)
		before, err := PollSchedulerQueue(ctx, tx, "tenant", expiry.Add(-time.Nanosecond))
		if err != nil || before.Next != "item" || len(before.Expired) != 0 {
			t.Fatalf("one nanosecond before expiry: %+v err=%v", before, err)
		}
		at, err := PollSchedulerQueue(ctx, tx, "tenant", expiry)
		if err != nil || at.Found || !slices.Equal(expiredIDs(at), []string{"item"}) {
			t.Fatalf("at expiry: %+v err=%v", at, err)
		}
	})
}

func TestAnUnreadableSchedulerTimeIsolatesOnlyThatRow(t *testing.T) {
	t.Parallel()
	for _, column := range []string{"created_at", "not_before", "expires_at"} {
		t.Run(column, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				enqueue(t, ctx, tx, queued{id: "a-bad", notBefore: 1, expires: 60}, queued{id: "b-good", expires: 60})
				execSQL(t, ctx, raw, "UPDATE scheduler_items SET "+column+" = 'not a time' WHERE scheduler_item_id = 'a-bad'")
				at := minutes(10)
				poll, err := PollSchedulerQueue(ctx, tx, "tenant", at)
				want := domain.ExpiredItem{SchedulerItemID: "a-bad", Reason: "unreadable " + column}
				if err != nil || poll.Next != "b-good" || len(poll.Expired) != 1 || poll.Expired[0] != want {
					t.Fatalf("poll = %+v err=%v, want next b-good and %+v expired", poll, err, want)
				}
				if got := dueIDs(t, ctx, tx, at); !slices.Equal(got, []string{"b-good"}) {
					t.Fatalf("replay selection = %v, want only the readable item", got)
				}
				must(t, ExpireSchedulerItem(ctx, tx, "a-bad", at))
				if again, err := PollSchedulerQueue(ctx, tx, "tenant", at); err != nil || again.Next != "b-good" || len(again.Expired) != 0 {
					t.Fatalf("second poll = %+v err=%v", again, err)
				}
			})
		})
	}
}

func admitInQueueOrder(t *testing.T, ctx context.Context, tx *store.Tx, at time.Time) []string {
	t.Helper()
	var order []string
	for {
		poll, err := PollSchedulerQueue(ctx, tx, "tenant", at)
		must(t, err)
		if !poll.Found {
			return order
		}
		must(t, MarkSchedulerItemAdmitted(ctx, tx, poll.Next, at))
		order = append(order, poll.Next)
	}
}

func TestLiveAdmissionOrderEqualsTheReplaySelection(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		enqueue(t, ctx, tx,
			queued{id: "d-debounced", notBefore: 5, expires: 60},
			queued{id: "c-later", created: 2, expires: 60},
			queued{id: "b-second", expires: 60},
			queued{id: "a-first", expires: 60},
			queued{id: "e-future", created: 20, expires: 60},
			queued{id: "f-dead-window", notBefore: 6, expires: 6},
		)
		at := minutes(10)
		want := []string{"a-first", "b-second", "c-later", "d-debounced"}
		if got := dueIDs(t, ctx, tx, at); !slices.Equal(got, want) {
			t.Fatalf("replay selection = %v, want %v", got, want)
		}
		if got := admitInQueueOrder(t, ctx, tx, at); !slices.Equal(got, want) {
			t.Fatalf("live admission order = %v, want %v", got, want)
		}
	})
}

func TestLiveAdmissionSkipsItemsPastTheirExpiry(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		enqueue(t, ctx, tx, queued{id: "a-stale", expires: 12}, queued{id: "b-fresh", expires: 60})
		at := minutes(15)
		if got := dueIDs(t, ctx, tx, at); !slices.Equal(got, []string{"a-stale", "b-fresh"}) {
			t.Fatalf("window selection = %v, want both items in the admission window", got)
		}
		if got := admitInQueueOrder(t, ctx, tx, at); !slices.Equal(got, []string{"b-fresh"}) {
			t.Fatalf("live admission = %v, want only the unexpired item", got)
		}
	})
}

func TestTheQueueOfOneTenantIsInvisibleToAnother(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		enqueue(t, ctx, tx, queued{id: "item", expires: 60})
		poll, err := PollSchedulerQueue(ctx, tx, "other", minutes(1))
		if err != nil || poll.Found || len(poll.Expired) != 0 {
			t.Fatalf("poll of another tenant = %+v err=%v, want nothing", poll, err)
		}
		due, err := DueSchedulerItems(ctx, tx, "other", minutes(1))
		if err != nil || len(due) != 0 {
			t.Fatalf("due items of another tenant = %v err=%v", due, err)
		}
	})
}

func TestCoalescingATriggerFlipsItsOpenItemsAndReportsTheirVersions(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		items := []queued{{id: "item-b", expires: 60}, {id: "item-a", expires: 60}}
		enqueue(t, ctx, tx, items...)
		for _, item := range items {
			recordAdmittedTrigger(t, ctx, raw, item)
		}
		must(t, MarkSchedulerItemAdmitted(ctx, tx, "item-a", now))
		got, err := CoalesceSchedulerItems(ctx, tx, "s", "alarm", now)
		want := []domain.CoalescedItem{{SchedulerItemID: "item-a", SituationVersion: 1}, {SchedulerItemID: "item-b", SituationVersion: 1}}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("coalesced = %v err=%v, want %v in identity order", got, err, want)
		}
		if statuses := itemStatus(t, ctx, raw, "item-a") + "|" + itemStatus(t, ctx, raw, "item-b"); statuses != "coalesced|coalesced" {
			t.Fatalf("statuses = %s", statuses)
		}
	})
}
