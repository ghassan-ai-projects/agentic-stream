package episodeledger_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func queueDB(t *testing.T, triggers ...string) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	for _, id := range triggers {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO trigger_evaluations (
 trigger_id,tenant_id,deployment_id,trigger_name,situation_id,situation_version,score,threshold,lane,outcome,reasons_json,policy_sha256,evaluated_at
 ) VALUES (?,'tenant','deployment','alarm','situation',1,1,0,'fast','admitted',X'5B5D',?,'now')`, id, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func schedulerItem(id, trigger string, expiresIn time.Duration) episodeledger.SchedulerItem {
	return episodeledger.SchedulerItem{SchedulerItemID: id, TriggerID: trigger, SituationID: "situation", SituationVersion: 1, Kind: episodeledger.KindStandard, Lane: "fast", Status: "pending", ExpiresAt: now.Add(expiresIn)}
}

func TestTheSchedulerQueueRunsEveryOperationThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := queueDB(t, "trg-a", "trg-b", "trg-c", "trg-d", "trg-e")
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		for i, item := range []episodeledger.SchedulerItem{
			schedulerItem("item-a", "trg-a", time.Hour), schedulerItem("item-b", "trg-b", time.Hour), schedulerItem("item-c", "trg-c", time.Hour),
			schedulerItem("item-d", "trg-d", time.Hour), schedulerItem("item-e", "trg-e", time.Minute),
		} {
			if err := episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", append([]byte{byte(i + 1)}, make([]byte, 31)...), now); err != nil {
				return err
			}
		}
		return nil
	})
	later := now.Add(10 * time.Minute)
	var due []episodeledger.DueItem
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		due, err = episodeledger.DueSchedulerItems(ctx, tx, "tenant", later)
		return err
	})
	if len(due) != 5 || due[0].SchedulerItemID != "item-a" || !due[0].AdmitAt.Equal(now) {
		t.Fatalf("due items = %+v, want the five in queue order, the first admittable at its creation", due)
	}
	poll, err := episodeledger.PollSchedulerQueue(t.Context(), db.DB, "tenant", later)
	if err != nil || poll.Next != "item-a" || !slices.Equal(poll.Expired, []episodeledger.ExpiredItem{{SchedulerItemID: "item-e", Reason: "expired before admission"}}) {
		t.Fatalf("poll = %+v err=%v, want item-a next and item-e expired", poll, err)
	}
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return errors.Join(
			episodeledger.ExpireSchedulerItem(ctx, tx, "item-e", later),
			episodeledger.MarkSchedulerItemAdmitted(ctx, tx, "item-a", later),
			episodeledger.CoalesceSkippedItem(ctx, tx, "item-b", later),
			episodeledger.CoalesceCostRejectedItem(ctx, tx, "item-c", later),
		)
	})
	var states string
	if err := db.QueryRowContext(t.Context(), "SELECT group_concat(status, ',') FROM (SELECT status FROM scheduler_items ORDER BY scheduler_item_id)").Scan(&states); err != nil || states != "admitted,coalesced,coalesced,pending,expired" {
		t.Fatalf("item states = %q err=%v", states, err)
	}
	var coalesced []episodeledger.CoalescedItem
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		coalesced, err = episodeledger.CoalesceSchedulerItems(ctx, tx, "situation", "alarm", later.Add(time.Minute))
		return err
	})
	if len(coalesced) != 2 || coalesced[0].SchedulerItemID != "item-a" || coalesced[1].SchedulerItemID != "item-d" {
		t.Fatalf("coalesced = %+v, want the open admitted and pending items", coalesced)
	}
}

func TestQueueAdmissionAndCoalescingShareTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := queueDB(t, "trigger")
	item := schedulerItem("item", "trigger", time.Hour)
	item.NotBefore = &now
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now)
	})
	rollback := errors.New("later participant failed")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		ctx := t.Context()
		if err := episodeledger.MarkSchedulerItemAdmitted(ctx, tx, "item", now); err != nil {
			return err
		}
		if _, err := episodeledger.CoalesceSchedulerItems(ctx, tx, "situation", "alarm", now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("composed rollback: %v", err)
	}
	var status, notBefore string
	if err := db.QueryRowContext(t.Context(), "SELECT status, not_before FROM scheduler_items WHERE scheduler_item_id = 'item'").Scan(&status, &notBefore); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || notBefore != kernel.FormatTime(now) {
		t.Fatalf("queue write escaped rollback: status=%s not-before=%s", status, notBefore)
	}
}
