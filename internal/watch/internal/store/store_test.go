package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
)

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func openStore(t *testing.T) Store {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, allowOwner, "epoch", interlock.DurableReader{})
}

func inTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := s.WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func condition() domain.Condition {
	return domain.Condition{TenantID: "tenant", SituationID: "sit-1", Expression: "features.x > 1", Target: "motor-1",
		ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339Nano), SituationVersion: 1, MaxFires: 2}
}

func TestStoreRequiresEverySafetyPort(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	reader := interlock.DurableReader{}
	for name, bad := range map[string]Store{
		"database": New(nil, allowOwner, "epoch", reader), "owner": New(s.db, nil, "epoch", reader), "interlock": New(s.db, allowOwner, "epoch", nil),
	} {
		if bad.Configured() {
			t.Fatalf("store without %s reported configured", name)
		}
	}
	if !s.Configured() {
		t.Fatal("complete store reported unconfigured")
	}
}

func TestConditionRoundTripsAndInsertIsIdempotent(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.LoadCondition(t.Context(), "w-1"); err != nil || found {
			t.Fatalf("missing watch found=%v err=%v", found, err)
		}
		for range 2 {
			if err := tx.InsertCondition(t.Context(), "w-1", condition(), testNow); err != nil {
				return err
			}
		}
		stored, found, err := tx.LoadCondition(t.Context(), "w-1")
		if err != nil || !found || stored != condition() {
			t.Fatalf("stored = %+v found=%v err=%v", stored, found, err)
		}
		return nil
	})
}

func TestFireIsRecordedOncePerEventAndSpendsAllowance(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	inTx(t, s, func(tx *Tx) error {
		first, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow)
		again, againErr := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow)
		if err != nil || againErr != nil || !first || again {
			t.Fatalf("first=%v again=%v errs=%v %v", first, again, err, againErr)
		}
		return tx.SpendAllowance(t.Context(), "w-1", testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-2", testNow); err != nil || !recorded {
			t.Fatalf("second event recorded=%v err=%v", recorded, err)
		}
		if err := tx.SpendAllowance(t.Context(), "w-1", testNow); err != nil {
			return err
		}
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-3", testNow); err != nil || recorded {
			t.Fatalf("exhausted watch recorded=%v err=%v", recorded, err)
		}
		_, found, err := tx.LoadActive(t.Context(), "w-1", testNow)
		if err != nil || found {
			t.Fatalf("disabled watch still active: found=%v err=%v", found, err)
		}
		return nil
	})
}

func TestExpiryHidesWatchesFromActiveReads(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	if candidates, err := s.ActiveForTarget(t.Context(), "motor-1", testNow); err != nil || len(candidates) != 1 || candidates[0] != (domain.Candidate{WatchID: "w-1", SituationID: "sit-1"}) {
		t.Fatalf("candidates = %+v err=%v", candidates, err)
	}
	if candidates, err := s.ActiveForTarget(t.Context(), "other", testNow); err != nil || len(candidates) != 0 {
		t.Fatalf("other target candidates = %+v err=%v", candidates, err)
	}
	later := testNow.Add(2 * time.Hour)
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.LoadActive(t.Context(), "w-1", later); err != nil || found {
			t.Fatalf("expired watch active before marking: found=%v err=%v", found, err)
		}
		return tx.ExpireDue(t.Context(), later)
	})
	inTx(t, s, func(tx *Tx) error {
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow); err != nil || recorded {
			t.Fatalf("expired watch recorded a fire: %v %v", recorded, err)
		}
		return nil
	})
}

func TestOwnerAndInterlockChecksRunOnTheTransaction(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lost := errors.New("ownership lost")
	denied := New(s.db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch", interlock.DurableReader{})
	if err := denied.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) }); !errors.Is(err, lost) {
		t.Fatalf("owner err = %v", err)
	}
	inTx(t, s, func(tx *Tx) error { return tx.AssertInterlock(t.Context(), "tenant", "motor-1") })
	if err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "stop", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertInterlock(t.Context(), "tenant", "motor-1") }); err == nil {
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

func TestContentionClassification(t *testing.T) {
	t.Parallel()
	if IsContended(errors.New("plain")) {
		t.Fatal("plain error classified as contention")
	}
}
