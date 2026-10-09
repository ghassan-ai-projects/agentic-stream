package store_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

func TestStoreReportsConfigurationAndOpenness(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	if store.New(nil).Configured() || !persistence.Configured() {
		t.Fatal("configuration report changed")
	}
	if store.Join(nil).Open() || !persistence.Autocommit().Open() {
		t.Fatal("openness report changed")
	}
}

func TestOwnerLeaseClaimRenewReleaseAndHold(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	later := instant.Add(time.Minute)
	if err := tx.ClaimLease(t.Context(), "e1", "i1", instant, later); err != nil {
		t.Fatal(err)
	}
	if epoch, instance, err := tx.RecordedOwner(t.Context()); err != nil || epoch != "e1" || instance != "i1" {
		t.Fatalf("recorded = %s %s %v", epoch, instance, err)
	}
	if held, err := tx.HoldsLease(t.Context(), "e1", "i1", instant); err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	if held, err := tx.HoldsLease(t.Context(), "e2", "i1", instant); err != nil || held {
		t.Fatalf("other epoch holds the lease: held=%v err=%v", held, err)
	}
	if rows, err := tx.RenewLease(t.Context(), "e1", "i1", instant, later); err != nil || rows != 1 {
		t.Fatalf("renew rows=%d err=%v", rows, err)
	}
	if rows, err := tx.RenewLease(t.Context(), "e2", "i1", instant, later); err != nil || rows != 0 {
		t.Fatalf("other epoch renewed: rows=%d err=%v", rows, err)
	}
	if rows, err := tx.ReleaseLease(t.Context(), "e1", "i1", instant); err != nil || rows != 1 {
		t.Fatalf("release rows=%d err=%v", rows, err)
	}
	if held, err := tx.HoldsLease(t.Context(), "e1", "i1", instant); err != nil || held {
		t.Fatalf("released lease still held: held=%v err=%v", held, err)
	}
}

func TestRecoverHandsTheClaimingTransactionToTheCaller(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	boom := errors.New("recovery failed")
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return tx.Recover(func(raw *sql.Tx, _ time.Time) error {
			if raw == nil {
				t.Error("recovery received no transaction")
			}
			return boom
		}, instant)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestHoldsLeaseJudgesEpochInstanceAndInstantTogether(t *testing.T) {
	t.Parallel()
	until := instant.Add(500 * time.Millisecond)
	for name, tc := range map[string]struct {
		epoch, instance string
		at              time.Time
		want            bool
	}{
		"owner before the lease ends":               {"e1", "i1", until.Add(-time.Nanosecond), true},
		"owner at a whole second before a fraction": {"e1", "i1", instant, true},
		"owner exactly at the lease end":            {"e1", "i1", until, false},
		"owner after the lease":                     {"e1", "i1", instant.Add(time.Second), false},
		"same epoch, another instance":              {"e1", "i2", instant, false},
		"another epoch, same instance":              {"e2", "i1", instant, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			persistence, _ := openStore(t)
			tx := persistence.Autocommit()
			if err := tx.ClaimLease(t.Context(), "e1", "i1", instant, until); err != nil {
				t.Fatal(err)
			}
			held, err := tx.HoldsLease(t.Context(), tc.epoch, tc.instance, tc.at)
			if err != nil || held != tc.want {
				t.Fatalf("held=%v err=%v, want %v", held, err, tc.want)
			}
		})
	}
}

func TestUnreadableLeaseTextIsNeverHeldOrRenewedAndCanBeClaimed(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{
		"empty":            "",
		"offset":           "2999-01-01T00:00:00.000000000+02:00",
		"space separated":  "2999-01-01 00:00:00.000000000Z",
		"trimmed fraction": "2999-01-01T00:00:00Z",
		"digits only":      "9999-99-99T99:99:99.999999999Z",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			persistence, db := openStore(t)
			tx := persistence.Autocommit()
			if err := tx.ClaimLease(t.Context(), "e1", "i1", instant, instant.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), "UPDATE runtime_owner SET lease_until = ?", corrupt); err != nil {
				t.Fatal(err)
			}
			if held, err := tx.HoldsLease(t.Context(), "e1", "i1", instant); err != nil || held {
				t.Fatalf("held=%v err=%v, want not held", held, err)
			}
			if rows, err := tx.RenewLease(t.Context(), "e1", "i1", instant, instant.Add(time.Hour)); err != nil || rows != 0 {
				t.Fatalf("renewed %d rows err=%v, want 0", rows, err)
			}
			if err := tx.ClaimLease(t.Context(), "e2", "i2", instant, instant.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if epoch, _, err := tx.RecordedOwner(t.Context()); err != nil || epoch != "e2" {
				t.Fatalf("owner after claim = %q err=%v, want e2", epoch, err)
			}
		})
	}
}
