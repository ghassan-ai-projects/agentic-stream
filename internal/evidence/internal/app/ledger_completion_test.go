package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func TestLedgerCompletionAndFailureSurviveARequestCancellation(t *testing.T) {
	t.Parallel()
	t.Run("completion", func(t *testing.T) {
		t.Parallel()
		ledger, db := newLedger(t)
		reservation := reserveTestCall(t, ledger)
		if err := ledger.Complete(canceledContext(t), reservation, QueryResult{JSON: []byte(`[]`)}); err != nil {
			t.Fatalf("complete with a canceled request: %v", err)
		}
		if status, _ := ledgerStatus(t, db, "call-1"); status != "completed" {
			t.Fatalf("status = %q, want completed", status)
		}
	})
	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		ledger, db := newLedger(t)
		reservation := reserveTestCall(t, ledger)
		if err := ledger.Fail(canceledContext(t), reservation, "provider_timeout"); err != nil {
			t.Fatalf("fail with a canceled request: %v", err)
		}
		if status, code := ledgerStatus(t, db, "call-1"); status != "failed" || code != "provider_timeout" {
			t.Fatalf("ledger = %q/%q, want failed/provider_timeout", status, code)
		}
	})
}

func TestLedgerTerminalCallsAcceptNoSecondOutcome(t *testing.T) {
	t.Parallel()
	result := QueryResult{JSON: []byte(`[]`)}
	t.Run("failed twice", func(t *testing.T) {
		t.Parallel()
		ledger, _ := newLedger(t)
		reservation := reserveTestCall(t, ledger)
		if err := ledger.Fail(t.Context(), reservation, "first"); err != nil {
			t.Fatal(err)
		}
		requireContains(t, ledger.Fail(t.Context(), reservation, "second"), "fail evidence call transaction")
	})
	t.Run("completed after failure", func(t *testing.T) {
		t.Parallel()
		ledger, db := newLedger(t)
		reservation := reserveTestCall(t, ledger)
		if err := ledger.Fail(t.Context(), reservation, "first"); err != nil {
			t.Fatal(err)
		}
		requireContains(t, ledger.Complete(t.Context(), reservation, result), "complete evidence call transaction")
		if status, _ := ledgerStatus(t, db, "call-1"); status != "failed" {
			t.Fatalf("status = %q, want failed", status)
		}
	})
	t.Run("failed after completion", func(t *testing.T) {
		t.Parallel()
		ledger, db := newLedger(t)
		reservation := reserveTestCall(t, ledger)
		if err := ledger.Complete(t.Context(), reservation, result); err != nil {
			t.Fatal(err)
		}
		requireContains(t, ledger.Fail(t.Context(), reservation, "late"), "fail evidence call transaction")
		if status, _ := ledgerStatus(t, db, "call-1"); status != "completed" {
			t.Fatalf("status = %q, want completed", status)
		}
	})
}

func TestLedgerRefusesOutcomesWithoutConfiguration(t *testing.T) {
	t.Parallel()
	reservation := ledgerReservation{}
	var missing *Ledger
	requireContains(t, missing.Complete(t.Context(), reservation, QueryResult{}), "ledger is not configured")
	requireContains(t, missing.Fail(t.Context(), reservation, "x"), "ledger is not configured")
	unconfigured := &Ledger{}
	requireContains(t, unconfigured.Complete(t.Context(), reservation, QueryResult{}), "ledger is not configured")
	requireContains(t, unconfigured.Fail(t.Context(), reservation, "x"), "ledger is not configured")
}

func TestLedgerOwnerLossRefusesCompletionAndFailure(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	reject := false
	ledger := ledgerOn(db, func(context.Context, *sql.Tx, string) error {
		if reject {
			return lost
		}
		return nil
	}, "epoch-1")
	reservation := reserveTestCall(t, ledger)

	reject = true
	if err := ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`)}); !errors.Is(err, lost) {
		t.Fatalf("complete error = %v, want the owner loss", err)
	}
	if err := ledger.Fail(t.Context(), reservation, "x"); !errors.Is(err, lost) {
		t.Fatalf("fail error = %v, want the owner loss", err)
	}
	if status, _ := ledgerStatus(t, db, "call-1"); status != "running" {
		t.Fatalf("status = %q, want running", status)
	}
}

func TestLedgerRefusesToCompleteForAnAttemptThatIsNoLongerCurrentAndRunning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		statement string
		want      string
	}{
		{"superseded episode", "UPDATE episodes SET lifecycle_status = 'superseded'", "no longer current"},
		{"concluded episode", "UPDATE episodes SET lifecycle_status = 'concluded'", "no longer current"},
		{"newer fence on the episode", "UPDATE episodes SET current_fence = 2", "no longer current"},
		{"failed attempt", "UPDATE episode_attempts SET status = 'failed'", "no longer active"},
		{"produced attempt", "UPDATE episode_attempts SET status = 'produced'", "no longer active"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ledger, db := newLedger(t)
			reservation := reserveTestCall(t, ledger)
			execSQL(t, db, test.statement)
			requireContains(t, ledger.Complete(t.Context(), reservation, QueryResult{JSON: []byte(`[]`)}), test.want)
			if status, _ := ledgerStatus(t, db, "call-1"); status != "running" {
				t.Fatalf("status = %q, want running", status)
			}
		})
	}
}
