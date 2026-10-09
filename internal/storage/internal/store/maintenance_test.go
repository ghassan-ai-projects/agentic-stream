package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestOpenFreshReservesPathUntilClose(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "replay.db")

	db, err := storage.OpenFresh(ctx, path)
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	if _, err := storagetest.Open(ctx, path); err == nil {
		t.Fatal("Open succeeded on a path reserved by an active replay")
	}
	if _, err := storage.OpenFresh(ctx, path); err == nil {
		t.Fatal("a second fresh open reused a reserved path")
	}
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("reservation not released: %v", err)
	}
	if _, err := storage.OpenFresh(ctx, path); err == nil {
		t.Fatal("OpenFresh accepted an existing database file")
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("failed fresh open leaked its reservation: %v", err)
	}
}

func TestRetrySQLiteBusyPassesThroughOtherOutcomes(t *testing.T) {
	t.Parallel()

	calls := 0
	if err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("success: err=%v calls=%d", err, calls)
	}
	permanent := errors.New("constraint failed")
	calls = 0
	if err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return permanent }); !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("non-busy errors must not retry: err=%v calls=%d", err, calls)
	}
	if storage.IsSQLiteBusy(permanent) || storage.IsSQLiteBusy(nil) {
		t.Fatal("non-SQLite error classified as busy")
	}
}

func TestRetrySQLiteBusyRetriesRealContention(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "busy.db")
	db, err := storagetest.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	holder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('held', 'draining', 'now')`); err != nil {
		t.Fatal(err)
	}

	contender, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contender.Close() })

	attempts := 0
	released := false
	err = storage.RetrySQLiteBusy(ctx, func() error {
		attempts++
		if attempts == 2 && !released {
			released = true
			if err := holder.Commit(); err != nil {
				t.Errorf("release writer lock: %v", err)
			}
		}
		tx, err := contender.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		return tx.Rollback()
	})
	if err != nil {
		t.Fatalf("retry did not recover from contention: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d; the first attempt should have hit a busy writer lock", attempts)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	busy := 0
	if err := storage.RetrySQLiteBusy(canceledCtx, func() error {
		busy++
		lock, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Rollback() }()
		if _, err := lock.ExecContext(ctx, `INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('held-2', 'draining', 'now')`); err != nil {
			return err
		}
		tx, err := contender.BeginTx(ctx, nil)
		if err == nil {
			_ = tx.Rollback()
		}
		return err
	}); !errors.Is(err, context.Canceled) || busy != 1 {
		t.Fatalf("canceled retry: err=%v attempts=%d", err, busy)
	}
}
