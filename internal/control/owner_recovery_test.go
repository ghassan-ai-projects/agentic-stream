package control_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func assertNothingSurvivedRollback(t *testing.T, db *storage.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Errorf("rollback left %s with count=%d err=%v", table, count, err)
		}
	}
}

func TestRecoveryThatFailsRollsBackTheNewClaimAndKeepsTheOldOwner(t *testing.T) {
	t.Parallel()
	db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
	first, second := ownerOn(db, "instance-1", clock), ownerOn(db, "instance-2", clock)
	if err := first.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	clock.Advance(2 * time.Minute)
	failure := errors.New("injected recovery failure")
	if err := second.ClaimAndRecover(t.Context(), "epoch-2", func(*sql.Tx, time.Time) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("claim and recovery = %v, want the injected failure", err)
	}
	if err := fenced(t, db, second.Assert, "epoch-2"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("the failed claim left epoch-2 owning the lease: %v", err)
	}
	if err := first.Renew(t.Context(), "epoch-1"); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("the expired lease of epoch-1 must not be revived by a rolled-back claim: %v", err)
	}
}

func TestRecoveryCannotCommitOnceTheOwnerLeaseIsLost(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, clock *sources.Virtual, tx *sql.Tx) error{
		"the lease row is rewritten as expired": func(t *testing.T, _ *sources.Virtual, tx *sql.Tx) error {
			t.Helper()
			_, err := tx.ExecContext(t.Context(), "UPDATE runtime_owner SET lease_until = ?", "2000-01-01T00:00:00.000000000Z")
			return err
		},
		"the clock passes the lease end": func(t *testing.T, clock *sources.Virtual, tx *sql.Tx) error {
			t.Helper()
			_, err := tx.ExecContext(t.Context(), `INSERT INTO epoch_control VALUES ('recovery-write', 'draining', 'now')`)
			clock.Advance(time.Minute)
			return err
		},
	}
	for name, loseLease := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, clock := openOwnerDB(t), sources.NewVirtual(epoch0)
			owner := ownerOn(db, "instance", clock)
			err := owner.ClaimAndRecover(t.Context(), "epoch", func(tx *sql.Tx, acquiredAt time.Time) error {
				if !acquiredAt.Equal(epoch0) {
					t.Errorf("recovery time = %v, want %v", acquiredAt, epoch0)
				}
				return loseLease(t, clock, tx)
			})
			if !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
				t.Fatalf("recovery commit = %v, want ErrRuntimeOwnerBusy", err)
			}
			assertNothingSurvivedRollback(t, db, "runtime_owner", "epoch_control")
		})
	}
}
