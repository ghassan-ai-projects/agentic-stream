package app

import (
	"context"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
)

func TestLedgerRejectsCorruptedCompletedResults(t *testing.T) {
	tests := []struct{ name, mutation, want string }{
		{"size", "result_bytes = -1", "stored evidence result is malformed"},
		{"row count", "row_count = -1", "stored evidence result is malformed"},
		{"digest size", "result_sha256 = X'00'", "stored evidence result is malformed"},
		{"digest mismatch", "result_sha256 = zeroblob(32)", "stored evidence result digest mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openLedgerDB(t)
			ledger := &Ledger{Store: store.New(db, allowOwner, "epoch-1"), LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Now: fixedLedgerClock()}
			call := ledgerTestCall()
			reservation, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1")
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`), RowCount: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), "PRAGMA ignore_check_constraints = ON"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), "UPDATE evidence_call_ledger SET "+tt.mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("reserve corrupted result = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestLedgerCompletionSurvivesCancellation(t *testing.T) {
	db := openLedgerDB(t)
	ledger := &Ledger{Store: store.New(db, allowOwner, "epoch-1"), LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Now: fixedLedgerClock()}
	reservation, err := ledger.Reserve(t.Context(), ledgerTestCall(), "token-1", "epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ledger.Complete(ctx, reservation, QueryResult{JSON: []byte(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if status, _ := readLedgerStatus(t, db, "call-1"); status != "completed" {
		t.Fatalf("status = %s", status)
	}
}

func TestReservationIdentityErrorsPrecedeResultIntegrity(t *testing.T) {
	db := openLedgerDB(t)
	ledger := &Ledger{Store: store.New(db, allowOwner, "epoch-1"), LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Now: fixedLedgerClock()}
	if _, err := ledger.Reserve(t.Context(), ledgerTestCall(), "original", "epoch-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA ignore_check_constraints = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE evidence_call_ledger SET status = 'completed', result_bytes = -1"); err != nil {
		t.Fatal(err)
	}
	changed := ledgerTestCall()
	changed.MaxBytes++
	if _, err := ledger.Reserve(t.Context(), changed, "other", "epoch-1"); err == nil || !strings.Contains(err.Error(), "different request") {
		t.Fatalf("error precedence = %v", err)
	}
}
