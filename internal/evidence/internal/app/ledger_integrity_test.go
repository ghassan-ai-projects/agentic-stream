package app

import "testing"

func TestLedgerRejectsCorruptedCompletedResults(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, mutation, want string }{
		{"size", "result_bytes = -1", "stored evidence result is malformed"},
		{"row count", "row_count = -1", "stored evidence result is malformed"},
		{"digest size", "result_sha256 = X'00'", "stored evidence result is malformed"},
		{"digest mismatch", "result_sha256 = zeroblob(32)", "stored evidence result digest mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ledger, db := newLedger(t)
			reservation := reserveTestCall(t, ledger)
			if err := ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`), RowCount: 1}); err != nil {
				t.Fatal(err)
			}
			execSQL(t, db, "PRAGMA ignore_check_constraints = ON")
			execSQL(t, db, "UPDATE evidence_call_ledger SET "+test.mutation)
			_, err := ledger.Reserve(t.Context(), ledgerTestCall(), "token-1", "epoch-1")
			requireContains(t, err, test.want)
		})
	}
}

func TestLedgerReportsReuseBeforeResultIntegrity(t *testing.T) {
	t.Parallel()
	ledger, db := newLedger(t)
	reserveTestCall(t, ledger)
	execSQL(t, db, "PRAGMA ignore_check_constraints = ON")
	execSQL(t, db, "UPDATE evidence_call_ledger SET status = 'completed', result_bytes = -1")
	changed := ledgerTestCall()
	changed.MaxBytes++
	_, err := ledger.Reserve(t.Context(), changed, "token-1", "epoch-1")
	requireContains(t, err, "different request")
}
