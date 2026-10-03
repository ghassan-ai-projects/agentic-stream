package control_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

func TestRecoveryCannotCommitAnExpiredOwnerLease(t *testing.T) {
	db, now := openOwnerDB(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance", Lease: time.Minute, Now: func() time.Time { return now }}
	err := owner.ClaimAndRecover(t.Context(), "epoch", func(tx *sql.Tx, _ time.Time) error {
		_, err := tx.ExecContext(t.Context(), "UPDATE runtime_owner SET lease_until = ?", "2000-01-01T00:00:00.000000000Z")
		return err
	})
	if !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("recovery commit = %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM runtime_owner").Scan(&count); err != nil || count != 0 {
		t.Fatalf("owner claim survived rollback: count=%d err=%v", count, err)
	}
}
