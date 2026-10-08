package episodeledger_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func queueDB(t *testing.T, triggers ...string) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	// Situation/spec provenance is covered by cognition acceptance tests; isolate queue transitions here.

	for _, id := range append([]string{"trigger", "other"}, triggers...) {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO trigger_evaluations (
 trigger_id,tenant_id,deployment_id,trigger_name,situation_id,situation_version,score,threshold,lane,outcome,reasons_json,policy_sha256,evaluated_at
 ) VALUES (?,'tenant','deployment','alarm','situation',1,1,0,'fast','admitted',X'5B5D',?,'now')`, id, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestQueueAdmissionAndCoalescingShareCallerTransaction(t *testing.T) {
	db := queueDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	item := episodeledger.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour), NotBefore: &now}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now)
	}); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("later participant failed")
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := episodeledger.MarkSchedulerItemAdmitted(ctx, tx, "item", now); err != nil {
			return err
		}
		if err := episodeledger.MarkSchedulerItemAdmitted(ctx, tx, "item", now); err == nil {
			t.Fatal("already admitted item was admitted twice")
		}
		if _, err := episodeledger.CoalesceSchedulerItems(ctx, tx, "situation", "alarm", now); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM scheduler_items WHERE scheduler_item_id='item'").Scan(&status); err != nil {
			return err
		}
		if status != "coalesced" {
			t.Fatalf("status=%s", status)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("composed rollback: %v", err)
	}
	var status, notBefore string
	if err := db.QueryRowContext(ctx, "SELECT status,not_before FROM scheduler_items WHERE scheduler_item_id='item'").Scan(&status, &notBefore); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || notBefore != kernel.FormatTime(now) {
		t.Fatalf("queue write escaped rollback: status=%s not-before=%s", status, notBefore)
	}
}

func TestUpsertPreservesDeterministicItemIdentityAcrossConflicts(t *testing.T) {
	db := queueDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	item := episodeledger.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour)}
	upsert := func(item episodeledger.SchedulerItem, key byte) error {
		digest := make([]byte, 32)
		digest[0] = key
		return db.WithTx(ctx, func(tx *sql.Tx) error {
			return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", digest, now)
		})
	}
	if err := upsert(item, 1); err != nil {
		t.Fatal(err)
	}
	collision := item
	collision.TriggerID = "other"
	if err := upsert(collision, 2); err != nil {
		t.Fatal(err)
	}
	replacement := item
	replacement.SchedulerItemID = "new-id"
	replacement.Priority = 9
	if err := upsert(replacement, 3); err != nil {
		t.Fatal(err)
	}
	var count int
	var id, trigger string
	var priority float64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*),scheduler_item_id,trigger_id,priority FROM scheduler_items").Scan(&count, &id, &trigger, &priority); err != nil {
		t.Fatal(err)
	}
	if count != 1 || id != "item" || trigger != "trigger" || priority != 9 {
		t.Fatalf("queue identity changed: count=%d id=%s trigger=%s priority=%v", count, id, trigger, priority)
	}
}

func TestSkippedOpportunitiesCoalesceOnlyWhilePending(t *testing.T) {
	for _, tc := range []struct {
		name string
		skip func(context.Context, *sql.Tx, string, time.Time) error
	}{{"cost rejection", episodeledger.CoalesceCostRejectedItem}, {"unavailable executor", episodeledger.CoalesceSkippedItem}} {
		t.Run(tc.name, func(t *testing.T) {
			db := queueDB(t)
			ctx := t.Context()
			now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			item := episodeledger.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour)}
			if err := db.WithTx(ctx, func(tx *sql.Tx) error {
				return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now)
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.WithTx(ctx, func(tx *sql.Tx) error { return tc.skip(ctx, tx, "item", now) }); err != nil {
				t.Fatal(err)
			}
			if err := db.WithTx(ctx, func(tx *sql.Tx) error { return tc.skip(ctx, tx, "item", now) }); err == nil {
				t.Fatal("non-pending opportunity was silently coalesced")
			}
			var state string
			if err := db.QueryRowContext(ctx, "SELECT status FROM scheduler_items WHERE scheduler_item_id='item'").Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "coalesced" {
				t.Fatalf("skipped state=%s", state)
			}
		})
	}
}

func TestCoalesceReturnsExactlyTheItemsItFlipped(t *testing.T) {
	db := queueDB(t, "trigger-b", "trigger-c", "trigger-d")
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	coalescedAt := now.Add(time.Minute)
	seed := func(id, trigger, situation, status string, version int, key byte) {
		item := episodeledger.SchedulerItem{SchedulerItemID: id, TriggerID: trigger, SituationID: situation, SituationVersion: version, Kind: "standard", Lane: "fast", Status: status, ExpiresAt: now.Add(time.Hour)}
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", append([]byte{key}, make([]byte, 31)...), now)
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed("item-c", "trigger-c", "situation", "pending", 3, 1)
	seed("item-a", "other", "situation", "admitted", 1, 2)
	seed("item-b", "trigger-b", "situation", "pending", 2, 3)
	seed("item-done", "trigger-d", "situation", "coalesced", 1, 4)
	seed("item-elsewhere", "trigger", "elsewhere", "pending", 1, 5)
	var got []episodeledger.CoalescedItem
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		got, err = episodeledger.CoalesceSchedulerItems(ctx, tx, "situation", "alarm", coalescedAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []episodeledger.CoalescedItem{{SchedulerItemID: "item-a", SituationVersion: 1}, {SchedulerItemID: "item-b", SituationVersion: 2}, {SchedulerItemID: "item-c", SituationVersion: 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("coalesced = %v, want %v", got, want)
	}
	flipped, err := storage.QueryAll(ctx, db, "flipped", func(r *sql.Rows) (string, error) {
		var id string
		return id, r.Scan(&id)
	}, "SELECT scheduler_item_id FROM scheduler_items WHERE updated_at = ? ORDER BY scheduler_item_id", kernel.FormatTime(coalescedAt))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(flipped, []string{"item-a", "item-b", "item-c"}) {
		t.Fatalf("flipped = %v", flipped)
	}
}

type queuedSeed struct {
	id                 string
	created            int
	notBefore, expires int
}

func seedQueue(t *testing.T, base time.Time, seeds []queuedSeed) *storage.DB {
	t.Helper()
	triggers := make([]string, len(seeds))
	for i, seed := range seeds {
		triggers[i] = "trigger-" + seed.id
	}
	db := queueDB(t, triggers...)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	for i, seed := range seeds {
		item := episodeledger.SchedulerItem{SchedulerItemID: seed.id, TriggerID: triggers[i], SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: at(seed.expires)}
		if seed.notBefore != 0 {
			notBefore := at(seed.notBefore)
			item.NotBefore = &notBefore
		}
		dedupeKey := sha256.Sum256([]byte(seed.id))
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return episodeledger.UpsertSchedulerItem(t.Context(), tx, item, "tenant", dedupeKey[:], at(seed.created))
		}); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func dueIDs(t *testing.T, db *storage.DB, now time.Time) []string {
	t.Helper()
	var ids []string
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		due, err := episodeledger.DueSchedulerItems(t.Context(), tx, "tenant", now)
		for _, item := range due {
			ids = append(ids, item.SchedulerItemID)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids
}

func admitInQueueOrder(t *testing.T, db *storage.DB, now time.Time) []string {
	t.Helper()
	var order []string
	for {
		poll, err := episodeledger.PollSchedulerQueue(t.Context(), db.DB, "tenant", now)
		if err != nil || !poll.Found {
			if err != nil {
				t.Fatal(err)
			}
			return order
		}
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return episodeledger.MarkSchedulerItemAdmitted(t.Context(), tx, poll.Next, now)
		}); err != nil {
			t.Fatal(err)
		}
		order = append(order, poll.Next)
	}
}

func TestLiveAdmissionOrderEqualsTheReplaySelection(t *testing.T) {
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	db := seedQueue(t, base, []queuedSeed{
		{id: "d-debounced", created: 0, notBefore: 5, expires: 60},
		{id: "c-later", created: 2, expires: 60},
		{id: "b-second", created: 0, expires: 60},
		{id: "a-first", created: 0, expires: 60},
		{id: "e-future", created: 20, expires: 60},
		{id: "f-dead-window", created: 0, notBefore: 6, expires: 6},
	})
	now := base.Add(10 * time.Minute)
	want := []string{"a-first", "b-second", "c-later", "d-debounced"}
	if got := dueIDs(t, db, now); !slices.Equal(got, want) {
		t.Fatalf("replay selection = %v, want %v", got, want)
	}
	if got := admitInQueueOrder(t, db, now); !slices.Equal(got, want) {
		t.Fatalf("live admission order = %v, want %v", got, want)
	}
}

func TestLiveAdmissionSkipsItemsPastTheirExpiry(t *testing.T) {
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	db := seedQueue(t, base, []queuedSeed{
		{id: "a-stale", created: 0, expires: 12},
		{id: "b-fresh", created: 0, expires: 60},
	})
	now := base.Add(15 * time.Minute)
	if got := dueIDs(t, db, now); !slices.Equal(got, []string{"a-stale", "b-fresh"}) {
		t.Fatalf("window selection = %v", got)
	}
	if got := admitInQueueOrder(t, db, now); !slices.Equal(got, []string{"b-fresh"}) {
		t.Fatalf("live admission = %v, want only the unexpired item", got)
	}
}
