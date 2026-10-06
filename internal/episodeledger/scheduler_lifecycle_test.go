package episodeledger_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func queueDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	// Situation/spec provenance is covered by cognition acceptance tests; isolate queue transitions here.
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"trigger", "other"} {
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
		return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now.Format(time.RFC3339Nano))
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
		if err := episodeledger.CoalesceSchedulerItems(ctx, tx, "situation", "alarm", "coalesced-at"); err != nil {
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
	if status != "pending" || notBefore != now.Format(time.RFC3339Nano) {
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
			return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", digest, "now")
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
				return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), "now")
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
