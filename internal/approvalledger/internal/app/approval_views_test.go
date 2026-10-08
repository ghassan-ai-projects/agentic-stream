package app

import (
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestApprovalsReadsWhatTheLifecycleRecorded(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		tx := store.Join(raw)
		if err := Request(t.Context(), tx, "apr-1", "int-1", "2026-10-08T00:00:00Z", "2026-10-08T01:00:00Z", []byte("{}"), "n-1"); err != nil {
			return err
		}
		if err := BindAssertion(t.Context(), tx, "apr-1", make([]byte, 32)); err != nil {
			return err
		}
		return Resolve(t.Context(), tx, "apr-1", "approved", "approver-1", "relay-1", "ok", "2026-10-08T00:10:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	approvals, err := Approvals(t.Context(), store.NewReader(db.DB), "int-1")
	if err != nil || len(approvals) != 1 || approvals[0].Status != "approved" || approvals[0].Approver != "approver-1" {
		t.Fatalf("approvals = %+v, %v", approvals, err)
	}
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error { return ExpireIntent(t.Context(), store.Join(raw), "int-1") }); err != nil {
		t.Fatal(err)
	}
}
