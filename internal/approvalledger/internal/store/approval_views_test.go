package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestApprovalsReadsEveryRequestOfAnIntent(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "approvals.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		tx := store.Join(raw)
		if err := tx.InsertPending(t.Context(), "apr-1", "int-1", "2026-10-08T00:00:00Z", "2026-10-08T01:00:00Z", []byte("{}"), "n-1"); err != nil {
			return err
		}
		return tx.DecidePending(t.Context(), "apr-1", "approved", "approver-1", "relay-1", "looks right", "2026-10-08T00:10:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	approvals, err := store.NewReader(db.DB).Approvals(t.Context(), "int-1")
	if err != nil || len(approvals) != 1 {
		t.Fatalf("approvals = %+v, %v", approvals, err)
	}
	if a := approvals[0]; a.Status != "approved" || a.Approver != "approver-1" || a.Relay != "relay-1" || a.Reason != "looks right" || a.WithdrawnAt != "" {
		t.Fatalf("approval = %+v", a)
	}
}
