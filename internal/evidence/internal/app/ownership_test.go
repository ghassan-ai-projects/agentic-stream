package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
)

func TestLedgerOwnerLossAndSupersessionRefuseCompletion(t *testing.T) {
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	reject := true
	ledger := &Ledger{Store: store.New(db, func(context.Context, *sql.Tx, string) error {
		if reject {
			return lost
		}
		return nil
	}, "epoch-1"), LeaseOwner: "owner", RuntimeEpoch: "epoch-1", Now: fixedLedgerClock()}
	call := ledgerTestCall()
	if _, err := ledger.Reserve(t.Context(), call, "token", "epoch-1"); !errors.Is(err, lost) {
		t.Fatalf("reserve=%v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM evidence_call_ledger").Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	reject = false
	reservation, err := ledger.Reserve(t.Context(), call, "token", "epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	reject = true
	if err := ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`)}); !errors.Is(err, lost) {
		t.Fatalf("complete=%v", err)
	}
	if status, _ := readLedgerStatus(t, db, "call-1"); status != "running" {
		t.Fatal(status)
	}
	reject = false
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = 'superseded'"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`)}); err == nil || !strings.Contains(err.Error(), "no longer current") {
		t.Fatalf("superseded=%v", err)
	}
	if status, _ := readLedgerStatus(t, db, "call-1"); status != "running" {
		t.Fatal(status)
	}
}
