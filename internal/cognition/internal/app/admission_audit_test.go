package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func auditDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	// Situation/spec provenance is covered by cognition acceptance tests; isolate queue transitions here.

	for _, id := range []string{"trigger", "other"} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO trigger_evaluations (
 trigger_id,tenant_id,deployment_id,trigger_name,situation_id,situation_version,score,threshold,lane,outcome,reasons_json,policy_sha256,evaluated_at
 ) VALUES (?,'tenant','deployment','alarm','situation',1,1,0,'fast','admitted',X'5B5D',?,'now')`, id, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func TestCostRejectionAuditUsesCallerTransactionAndLeavesQueueStateToOwner(t *testing.T) {
	db := auditDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return episodeledger.UpsertSchedulerItem(ctx, tx, episodeledger.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour)}, "tenant", make([]byte, 32), now)
	}); err != nil {
		t.Fatal(err)
	}
	rejection := errors.New("budget exhausted")
	rollback := errors.New("queue transition failed")
	record := func(ctx context.Context, tx *sql.Tx) error {
		return RecordCostRejectionReason(ctx, store.Join(tx), "item", rejection)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := record(ctx, tx); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("audit rollback: %v", err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, "SELECT reasons_json FROM trigger_evaluations WHERE trigger_id='trigger'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("audit escaped rollback: %s", raw)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return record(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT reasons_json FROM trigger_evaluations WHERE trigger_id='trigger'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var reasons []string
	if err := json.Unmarshal(raw, &reasons); err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 1 || !strings.Contains(reasons[0], rejection.Error()) {
		t.Fatalf("cost reason missing: %v", reasons)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM scheduler_items WHERE scheduler_item_id='item'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("audit mutated queue lifecycle: %s", status)
	}
}

func TestSchedulerExpiryReasonIsRecordedOnTheEvaluationWithoutTouchingTheQueue(t *testing.T) {
	db := auditDB(t)
	ctx := t.Context()
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return RecordSchedulerExpiryReason(ctx, store.Join(tx), "item-of-trigger", "unreadable expires_at")
	}); err == nil {
		t.Fatal("an item without a queue row was explained")
	}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return episodeledger.UpsertSchedulerItem(ctx, tx, episodeledger.SchedulerItem{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now}, "tenant", make([]byte, 32), now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return RecordSchedulerExpiryReason(ctx, store.Join(tx), "item", "unreadable expires_at")
	}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, "SELECT reasons_json FROM trigger_evaluations WHERE trigger_id='trigger'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != `["scheduler item expired: unreadable expires_at"]` {
		t.Fatalf("reasons = %s", raw)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return RecordSchedulerExpiryReason(ctx, store.Join(tx), "item", "") }); err == nil {
		t.Fatal("an empty reason was recorded")
	}
}

func TestOnlyPendingItemsCountTowardGlobalCapacity(t *testing.T) {
	db := auditDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	for _, item := range []episodeledger.SchedulerItem{
		{SchedulerItemID: "live", TriggerID: "trigger", Status: "pending"},
		{SchedulerItemID: "dead", TriggerID: "other", Status: "expired"},
	} {
		item.SituationID, item.SituationVersion, item.Kind, item.Lane, item.ExpiresAt = "situation", 1, "standard", "fast", now
		dedupeKey := sha256.Sum256([]byte(item.SchedulerItemID))
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			return episodeledger.UpsertSchedulerItem(ctx, tx, item, "tenant", dedupeKey[:], now)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var pending int
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		pending, err = store.Join(tx).CountPending(ctx, "tenant")
		return err
	}); err != nil || pending != 1 {
		t.Fatalf("pending count = %d, %v; want only the pending item", pending, err)
	}
}
