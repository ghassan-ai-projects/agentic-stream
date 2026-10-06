package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"

	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func auditDB(t *testing.T) *storage.DB {
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
func TestCostRejectionAuditUsesCallerTransactionAndLeavesQueueStateToOwner(t *testing.T) {
	db := auditDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return scheduleledger.Upsert(ctx, tx, scheduleledger.Item{SchedulerItemID: "item", TriggerID: "trigger", SituationID: "situation", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour)}, "tenant", make([]byte, 32), "now")
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
