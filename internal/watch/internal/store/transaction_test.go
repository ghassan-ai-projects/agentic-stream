package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestStoreRequiresEverySafetyPort(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	for name, bad := range map[string]Store{
		"database": New(nil, allowOwner, "epoch"), "owner": New(s.db, nil, "epoch"),
	} {
		if bad.Configured() {
			t.Fatalf("store without %s reported configured", name)
		}
	}
	if !s.Configured() {
		t.Fatal("complete store reported unconfigured")
	}
}

func TestOwnerAndInterlockChecksRunOnTheTransaction(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lost := errors.New("ownership lost")
	denied := New(s.db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch")
	if err := denied.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) }); !errors.Is(err, lost) {
		t.Fatalf("owner err = %v", err)
	}
	inTx(t, s, func(tx *Tx) error { return tx.AssertInterlock(t.Context()) })
	if err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "stop", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertInterlock(t.Context()) }); err == nil {
		t.Fatal("tripped interlock accepted a watch write")
	}
}

func TestFailedUnitOfWorkRollsBack(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	boom := errors.New("boom")
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.InsertCondition(t.Context(), "w-1", condition(), testNow); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.LoadCondition(t.Context(), "w-1"); err != nil || found {
			t.Fatalf("rolled-back watch found=%v err=%v", found, err)
		}
		return nil
	})
}
