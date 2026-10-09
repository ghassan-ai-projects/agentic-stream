package episodeledger_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func pollQueue(t *testing.T, db *storage.DB, now time.Time) episodeledger.QueuePoll {
	t.Helper()
	poll, err := episodeledger.PollSchedulerQueue(t.Context(), db.DB, "tenant", now)
	if err != nil {
		t.Fatal(err)
	}
	return poll
}

func expireAll(t *testing.T, db *storage.DB, poll episodeledger.QueuePoll, now time.Time) {
	t.Helper()
	for _, expired := range poll.Expired {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return episodeledger.ExpireSchedulerItem(t.Context(), tx, expired.SchedulerItemID, now)
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func statusOf(t *testing.T, db *storage.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func expiredIDs(poll episodeledger.QueuePoll) []string {
	var ids []string
	for _, item := range poll.Expired {
		ids = append(ids, item.SchedulerItemID)
	}
	return ids
}

func TestExpiredSchedulerItemsLeaveThePendingQueue(t *testing.T) {
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	db := seedQueue(t, base, []queuedSeed{
		{id: "a-stale", created: 0, expires: 12},
		{id: "b-fresh", created: 0, expires: 60},
		{id: "c-stale", created: 1, expires: 13},
	})
	now := base.Add(15 * time.Minute)
	poll := pollQueue(t, db, now)
	if poll.Next != "b-fresh" || !slices.Equal(expiredIDs(poll), []string{"a-stale", "c-stale"}) {
		t.Fatalf("poll = %+v", poll)
	}
	if poll.Expired[0].Reason != "expired before admission" {
		t.Fatalf("reason = %q", poll.Expired[0].Reason)
	}
	expireAll(t, db, poll, now)
	if status := statusOf(t, db, "a-stale"); status != "expired" {
		t.Fatalf("stale item status = %q", status)
	}
	again := pollQueue(t, db, now)
	if again.Next != "b-fresh" || len(again.Expired) != 0 {
		t.Fatalf("second poll rescans expired items: %+v", again)
	}
}

func TestSchedulerItemExpiresExactlyAtItsExpiryInstant(t *testing.T) {
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	db := seedQueue(t, base, []queuedSeed{{id: "item", created: 0, expires: 10}})
	expiry := base.Add(10 * time.Minute)
	if poll := pollQueue(t, db, expiry.Add(-time.Nanosecond)); poll.Next != "item" || len(poll.Expired) != 0 {
		t.Fatalf("one nanosecond before expiry: %+v", poll)
	}
	if poll := pollQueue(t, db, expiry); poll.Found || !slices.Equal(expiredIDs(poll), []string{"item"}) {
		t.Fatalf("at expiry: %+v", poll)
	}
}

func TestExpiringANonPendingSchedulerItemIsRefused(t *testing.T) {
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	db := seedQueue(t, base, []queuedSeed{{id: "item", created: 0, expires: 10}})
	expire := func() error {
		return db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return episodeledger.ExpireSchedulerItem(t.Context(), tx, "item", base)
		})
	}
	if err := expire(); err != nil {
		t.Fatal(err)
	}
	if err := expire(); err == nil {
		t.Fatal("an expired item was expired twice")
	}
}

func TestUnreadableSchedulerTimeIsolatesOnlyThatRow(t *testing.T) {
	for _, column := range []string{"created_at", "not_before", "expires_at"} {
		t.Run(column, func(t *testing.T) {
			base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			db := seedQueue(t, base, []queuedSeed{
				{id: "a-bad", created: 0, notBefore: 1, expires: 60},
				{id: "b-good", created: 0, expires: 60},
			})
			if _, err := db.ExecContext(t.Context(), "UPDATE scheduler_items SET "+column+" = 'not a time' WHERE scheduler_item_id = 'a-bad'"); err != nil {
				t.Fatal(err)
			}
			now := base.Add(10 * time.Minute)
			poll := pollQueue(t, db, now)
			if poll.Next != "b-good" || len(poll.Expired) != 1 || poll.Expired[0] != (episodeledger.ExpiredItem{SchedulerItemID: "a-bad", Reason: "unreadable " + column}) {
				t.Fatalf("poll = %+v", poll)
			}
			if got := dueIDs(t, db, now); !slices.Equal(got, []string{"b-good"}) {
				t.Fatalf("replay selection = %v, want only the readable item", got)
			}
			expireAll(t, db, poll, now)
			if status := statusOf(t, db, "a-bad"); status != "expired" {
				t.Fatalf("unreadable item status = %q", status)
			}
			if again := pollQueue(t, db, now); again.Next != "b-good" || len(again.Expired) != 0 {
				t.Fatalf("second poll = %+v", again)
			}
		})
	}
}
