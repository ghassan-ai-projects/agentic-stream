package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestLedgerRecoverTxInterruptsPriorEpochCallsOnly(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	previous := ledgerOn(db, allowOwner, "epoch-old")
	if _, err := previous.Reserve(t.Context(), ledgerTestCall(), "token-old", "epoch-old"); err != nil {
		t.Fatalf("reserve under the previous epoch: %v", err)
	}
	current := ledgerOn(db, allowOwner, "epoch-new")
	other := ledgerTestCall()
	other.CallID = "call-2"
	if _, err := current.Reserve(t.Context(), other, "token-new", "epoch-new"); err != nil {
		t.Fatalf("reserve under the current epoch: %v", err)
	}

	var recovered int
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		recovered, err = current.RecoverTx(t.Context(), current.Store.Join(tx), testNow)
		return err
	})
	if err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, err %v, want 1", recovered, err)
	}
	if status, code := ledgerStatus(t, db, "call-1"); status != "interrupted" || code != "runtime_restart" {
		t.Fatalf("prior-epoch call = %q/%q, want interrupted/runtime_restart", status, code)
	}
	if status, _ := ledgerStatus(t, db, "call-2"); status != "running" {
		t.Fatalf("current-epoch call = %q, want running", status)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		again, err := current.RecoverTx(t.Context(), current.Store.Join(tx), testNow)
		if again != 0 {
			t.Errorf("repeat recovery = %d, want 0", again)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLedgerRecoveryRollsBackWithTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	previous := ledgerOn(db, allowOwner, "epoch-old")
	if _, err := previous.Reserve(t.Context(), ledgerTestCall(), "token-old", "epoch-old"); err != nil {
		t.Fatal(err)
	}
	current := ledgerOn(db, allowOwner, "epoch-new")
	abort := errors.New("startup aborted")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := current.RecoverTx(t.Context(), current.Store.Join(tx), testNow); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("error = %v, want the caller's abort", err)
	}
	if status, _ := ledgerStatus(t, db, "call-1"); status != "running" {
		t.Fatalf("status = %q, want running after the caller rolled back", status)
	}
}

func TestLedgerRecoveryRefusesWithoutOwnershipOrConfiguration(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	t.Run("owner lost", func(t *testing.T) {
		t.Parallel()
		ledger := ledgerOn(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch-new")
		err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			_, err := ledger.RecoverTx(t.Context(), ledger.Store.Join(tx), testNow)
			return err
		})
		if !errors.Is(err, lost) {
			t.Fatalf("error = %v, want the owner loss", err)
		}
	})
	t.Run("unconfigured", func(t *testing.T) {
		t.Parallel()
		var missing *Ledger
		_, err := missing.RecoverTx(t.Context(), nil, testNow)
		requireContains(t, err, "recovery is not configured")
		ledger := ledgerOn(db, allowOwner, "epoch-new")
		_, err = ledger.RecoverTx(t.Context(), ledger.Store.Join(nil), testNow)
		requireContains(t, err, "recovery is not configured")
	})
}

func TestLedgerReclaimsExpiredLeasesWithTheConfiguredVirtualClock(t *testing.T) {
	t.Parallel()
	ledger, db := newLedger(t)
	clock := sources.NewVirtual(testNow)
	ledger.Now = clock.Now
	reserveTestCall(t, ledger)

	if err := ledger.ReclaimExpired(t.Context(), ledger.now()); err != nil {
		t.Fatalf("reclaim before lease expiry: %v", err)
	}
	if status, _ := ledgerStatus(t, db, "call-1"); status != "running" {
		t.Fatalf("status before lease expiry = %q, want running", status)
	}
	clock.Advance(2 * time.Minute)
	if err := ledger.ReclaimExpired(t.Context(), ledger.now()); err != nil {
		t.Fatalf("reclaim after lease expiry: %v", err)
	}
	if status, code := ledgerStatus(t, db, "call-1"); status != "interrupted" || code != "lease_expired" {
		t.Fatalf("ledger = %q/%q, want interrupted/lease_expired", status, code)
	}
}

func TestLedgerReclaimInterruptsCallsOfAForeignEpoch(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	previous := ledgerOn(db, allowOwner, "epoch-old")
	if _, err := previous.Reserve(t.Context(), ledgerTestCall(), "token-1", "epoch-old"); err != nil {
		t.Fatal(err)
	}
	current := ledgerOn(db, allowOwner, "epoch-new")
	if err := current.ReclaimExpired(t.Context(), testNow); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if status, code := ledgerStatus(t, db, "call-1"); status != "interrupted" || code != "lease_expired" {
		t.Fatalf("ledger = %q/%q, want interrupted/lease_expired", status, code)
	}
}

func TestLedgerReclaimRefusesWithoutOwnershipOrConfiguration(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	ledger := ledgerOn(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch-1")
	if err := ledger.ReclaimExpired(t.Context(), testNow); !errors.Is(err, lost) {
		t.Fatalf("error = %v, want the owner loss", err)
	}
	var missing *Ledger
	tests := map[string]*Ledger{"nil ledger": missing, "empty ledger": {}, "no database": {RuntimeEpoch: "epoch"}}
	for name, unconfigured := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireContains(t, unconfigured.ReclaimExpired(t.Context(), testNow), "ledger is not configured")
		})
	}
}
