package store_test

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

const schedulerRowSQL = `SELECT scheduler_item_id || '|' || trigger_id || '|' || status || '|' || priority || '|' || COALESCE(not_before, '-') || '|' || expires_at || '|' || created_at || '|' || updated_at
	FROM scheduler_items WHERE trigger_id = ?`

func TestAQueuedItemKeepsItsIdentityWhenItsTriggerIsQueuedAgain(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		must(t, tx.UpsertSchedulerItem(ctx, item("item", "trigger"), "tenant", make32(1), at))
		refreshed := item("item-new-id", "trigger")
		refreshed.Priority, refreshed.SituationVersion = 9, 2
		notBefore := at.Add(time.Minute)
		refreshed.NotBefore = &notBefore
		must(t, tx.UpsertSchedulerItem(ctx, refreshed, "tenant", make32(2), at.Add(time.Hour)))
		want := "item|trigger|pending|9.0|" + ts(time.Minute) + "|" + ts(time.Hour) + "|" + ts(0) + "|" + ts(time.Hour)
		if got := queryText(t, ctx, raw, schedulerRowSQL, "trigger"); got != want {
			t.Fatalf("refreshed item = %s, want %s", got, want)
		}
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) FROM scheduler_items"); count != "1" {
			t.Fatalf("scheduler items = %s, want the one queued item", count)
		}
	})
}

func TestAnItemIDClashWithAnotherTriggerFailsInsteadOfDroppingTheItem(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		must(t, tx.UpsertSchedulerItem(ctx, item("item", "trigger-1"), "tenant", make32(1), at))
		clash := item("item", "trigger-2")
		clash.Priority = 5
		if err := tx.UpsertSchedulerItem(ctx, clash, "tenant", make32(2), at); err == nil || !strings.Contains(err.Error(), "scheduler_items.scheduler_item_id") {
			t.Fatalf("id clash = %v, want the uniqueness failure reported", err)
		}
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) || '|' || MIN(trigger_id) || '|' || MIN(priority) FROM scheduler_items"); count != "1|trigger-1|0.0" {
			t.Fatalf("after the clash: %s, want the original item untouched", count)
		}
	})
}

func TestPendingTransitionsReportTheRowsTheyChange(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		leave func(*store.Tx, context.Context) (int64, error)
		state string
	}{
		"admit": {func(tx *store.Tx, ctx context.Context) (int64, error) {
			return tx.AdmitPendingSchedulerItem(ctx, "item", at)
		}, "admitted"},
		"coalesce": {func(tx *store.Tx, ctx context.Context) (int64, error) {
			return tx.CoalescePendingSchedulerItem(ctx, "item", at, "op")
		}, "coalesced"},
		"expire": {func(tx *store.Tx, ctx context.Context) (int64, error) {
			return tx.ExpirePendingSchedulerItem(ctx, "item", at)
		}, "expired"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				must(t, tx.UpsertSchedulerItem(ctx, item("item", "trigger"), "tenant", make32(1), at))
				rows, err := tc.leave(tx, ctx)
				if err != nil || rows != 1 {
					t.Fatalf("first transition rows=%d err=%v", rows, err)
				}
				if rows, err := tc.leave(tx, ctx); err != nil || rows != 0 {
					t.Fatalf("second transition rows=%d err=%v, want it refused as no longer pending", rows, err)
				}
				if got := queryText(t, ctx, raw, "SELECT status FROM scheduler_items WHERE scheduler_item_id = 'item'"); got != tc.state {
					t.Fatalf("status = %s, want %s", got, tc.state)
				}
			})
		})
	}
}

func TestAPendingTransitionOfAnUnknownItemChangesNothing(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if rows, err := tx.AdmitPendingSchedulerItem(ctx, "missing", at); err != nil || rows != 0 {
			t.Fatalf("admit rows=%d err=%v", rows, err)
		}
		if rows, err := tx.ExpirePendingSchedulerItem(ctx, "missing", at); err != nil || rows != 0 {
			t.Fatalf("expire rows=%d err=%v", rows, err)
		}
	})
}

func TestCoalescingATriggerFlipsOnlyItsOpenItemsAndReturnsThemInIdOrder(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		for _, trigger := range []string{"t-c", "t-a", "t-b", "t-done", "t-elsewhere", "t-rejected"} {
			outcome := "admitted"
			if trigger == "t-rejected" {
				outcome = "deferred"
			}
			execSQL(t, ctx, raw, `INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at)
				VALUES (?, 'tenant', 'deployment', 'alarm', ?, 1, 1, 0, 'fast', ?, X'5B5D', ?, 'now')`, trigger, situationOf(trigger), outcome, make32(1))
		}
		for key, seed := range []struct {
			id, trigger, status string
			version             int
		}{
			{"item-c", "t-c", "pending", 3}, {"item-a", "t-a", "admitted", 1}, {"item-b", "t-b", "pending", 2},
			{"item-done", "t-done", "completed", 1}, {"item-elsewhere", "t-elsewhere", "pending", 1}, {"item-rejected", "t-rejected", "pending", 1},
		} {
			queued := item(seed.id, seed.trigger)
			queued.SituationID, queued.SituationVersion, queued.Status = situationOf(seed.trigger), seed.version, seed.status
			must(t, tx.UpsertSchedulerItem(ctx, queued, "tenant", make32(byte(key+1)), at))
		}
		got, err := tx.CoalesceTriggerItems(ctx, "situation", "alarm", at.Add(time.Minute))
		want := []domain.CoalescedItem{{SchedulerItemID: "item-a", SituationVersion: 1}, {SchedulerItemID: "item-b", SituationVersion: 2}, {SchedulerItemID: "item-c", SituationVersion: 3}}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("coalesced = %v err=%v, want %v", got, err, want)
		}
		statuses := queryText(t, ctx, raw, "SELECT group_concat(scheduler_item_id || '=' || status, ',') FROM (SELECT * FROM scheduler_items ORDER BY scheduler_item_id)")
		if want := "item-a=coalesced,item-b=coalesced,item-c=coalesced,item-done=completed,item-elsewhere=pending,item-rejected=pending"; statuses != want {
			t.Fatalf("item statuses = %s, want %s", statuses, want)
		}
		if again, err := tx.CoalesceTriggerItems(ctx, "situation", "alarm", at); err != nil || len(again) != 0 {
			t.Fatalf("second coalesce = %v err=%v, want nothing left to flip", again, err)
		}
	})
}

func situationOf(trigger string) string {
	if trigger == "t-elsewhere" {
		return "elsewhere"
	}
	return "situation"
}

func TestThePendingQueueIsTenantScopedAndIsolatesUnreadableTimes(t *testing.T) {
	t.Parallel()
	for _, column := range []string{"created_at", "not_before", "expires_at"} {
		t.Run(column, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
				notBefore := at.Add(time.Minute)
				for key, id := range []string{"a-bad", "b-good", "c-admitted"} {
					queued := item(id, "trigger-"+id)
					queued.NotBefore = &notBefore
					must(t, tx.UpsertSchedulerItem(ctx, queued, "tenant", make32(byte(key+1)), at))
				}
				must(t, tx.UpsertSchedulerItem(ctx, item("d-other-tenant", "trigger-d"), "other", make32(9), at))
				execSQL(t, ctx, raw, "UPDATE scheduler_items SET status = 'admitted' WHERE scheduler_item_id = 'c-admitted'")
				execSQL(t, ctx, raw, "UPDATE scheduler_items SET "+column+" = 'not a time' WHERE scheduler_item_id = 'a-bad'")
				queue, err := tx.PendingQueue(ctx, "tenant")
				if err != nil {
					t.Fatal(err)
				}
				if want := []domain.UnreadableItem{{SchedulerItemID: "a-bad", Column: column}}; !slices.Equal(queue.Unreadable, want) {
					t.Fatalf("unreadable = %+v, want %+v", queue.Unreadable, want)
				}
				if len(queue.Items) != 1 || queue.Items[0].SchedulerItemID != "b-good" || !queue.Items[0].CreatedAt.Equal(at) ||
					queue.Items[0].NotBefore == nil || !queue.Items[0].NotBefore.Equal(notBefore) || !queue.Items[0].ExpiresAt.Equal(at.Add(time.Hour)) {
					t.Fatalf("readable items = %+v, want only the pending, readable item of the tenant", queue.Items)
				}
			})
		})
	}
}

func TestAnItemWithoutNotBeforeIsQueuedWithoutOne(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		must(t, tx.UpsertSchedulerItem(ctx, item("item", "trigger"), "tenant", make32(1), at))
		queue, err := tx.PendingQueue(ctx, "tenant")
		if err != nil || len(queue.Items) != 1 || queue.Items[0].NotBefore != nil || len(queue.Unreadable) != 0 {
			t.Fatalf("queue = %+v err=%v, want one item with no not_before", queue, err)
		}
	})
}
