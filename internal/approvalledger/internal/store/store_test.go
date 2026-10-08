package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "approvals.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw), raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func status(t *testing.T, ctx context.Context, raw *sql.Tx, id string) string {
	t.Helper()
	var state string
	if err := raw.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id = ?", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestEveryTransitionStartsFromPending(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		for _, id := range []string{"decided", "expired", "withdrawn", "intent"} {
			if err := tx.InsertPending(ctx, id, "i-"+id, "now", "later", []byte("{}"), "n-"+id); err != nil {
				t.Fatal(err)
			}
		}
		steps := []error{
			tx.DecidePending(ctx, "decided", "approved", "human", "relay", "ok", "t"),
			tx.ExpirePending(ctx, "expired", "t", "approval_expired"),
			tx.WithdrawPending(ctx, "withdrawn", "t", "situation_version_conflict", "approval_withdrawn"),
			tx.ExpirePendingOfIntent(ctx, "i-intent"),
			tx.BindAssertionDigest(ctx, "decided", make([]byte, 32)),
		}
		for i, err := range steps {
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		}
		for id, want := range map[string]string{"decided": "approved", "expired": "expired", "withdrawn": "denied", "intent": "expired"} {
			if got := status(t, ctx, raw, id); got != want {
				t.Errorf("%s = %s, want %s", id, got, want)
			}
		}
		// A later transition cannot revive a decided approval.
		if err := tx.WithdrawPending(ctx, "decided", "t2", "x", "y"); err != nil {
			t.Fatal(err)
		}
		if got := status(t, ctx, raw, "decided"); got != "approved" {
			t.Fatalf("decided approval changed to %s", got)
		}
	})
}

func TestSupersededApprovalsAreEmptyWithoutIntents(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		withdrawals, err := tx.SupersededApprovals(ctx, "situation", 2)
		if err != nil || len(withdrawals) != 0 {
			t.Fatalf("withdrawals=%v err=%v", withdrawals, err)
		}
	})
}
