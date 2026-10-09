package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestStoreAndJoinedTransactionReportWhetherTheyAreUsable(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	tests := []struct {
		name  string
		store Store
		want  bool
	}{
		{"complete", New(db, allowOwner, "epoch-1"), true},
		{"without a database", New(nil, allowOwner, "epoch-1"), false},
		{"without an owner assertion", New(db, nil, "epoch-1"), false},
		{"without an epoch", New(db, allowOwner, ""), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.store.Configured(); got != test.want {
				t.Fatalf("Configured() = %v, want %v", got, test.want)
			}
		})
	}
	t.Run("transaction handles", func(t *testing.T) {
		t.Parallel()
		s := New(db, allowOwner, "epoch-1")
		var nilTx *Tx
		if nilTx.Configured() || s.Join(nil).Configured() {
			t.Fatal("a missing transaction reported as usable")
		}
		mustInTx(t, s, func(tx *Tx) error {
			if !tx.Configured() {
				t.Error("an open transaction reported as unusable")
			}
			return nil
		})
	})
}

func TestOwnerAssertionRunsInTheSameTransactionWithTheStoreEpoch(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	var seenTx *sql.Tx
	var seenEpoch string
	s := New(db, func(_ context.Context, tx *sql.Tx, epoch string) error {
		seenTx, seenEpoch = tx, epoch
		return nil
	}, "epoch-7")
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return s.Join(tx).AssertOwner(t.Context()) }); err != nil {
		t.Fatal(err)
	}
	if seenTx == nil || seenEpoch != "epoch-7" {
		t.Fatalf("assertion saw tx %v epoch %q, want the joined transaction and epoch-7", seenTx, seenEpoch)
	}
}

func TestOwnerLossIsReportedWrappedAndEndsTheTransaction(t *testing.T) {
	t.Parallel()
	lost := errors.New("owner lost")
	s := New(openLedgerDB(t), func(context.Context, *sql.Tx, string) error { return lost }, "epoch-1")
	err := inTx(t, s, func(tx *Tx) error { return tx.AssertOwner(t.Context()) })
	if !errors.Is(err, lost) {
		t.Fatalf("error = %v, want the owner loss", err)
	}
}

func TestStoreRefusesWorkOnAClosedDatabase(t *testing.T) {
	t.Parallel()
	db := openLedgerDB(t)
	s := New(db, allowOwner, "epoch-1")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := inTx(t, s, func(*Tx) error { return nil }); err == nil {
		t.Fatal("a closed database accepted a transaction")
	}
}
