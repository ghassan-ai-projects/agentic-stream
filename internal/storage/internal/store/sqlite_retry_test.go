package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const insertHeldEpoch = `INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('held', 'draining', 'now')`

func TestRetrySQLiteBusyPassesThroughOtherOutcomes(t *testing.T) {
	t.Parallel()
	permanent := errors.New("constraint failed")
	tests := []struct {
		name      string
		result    error
		wantCalls int
	}{
		{"success", nil, 1},
		{"a non-busy failure is not retried", permanent, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return tc.result })
			if !errors.Is(err, tc.result) || calls != tc.wantCalls {
				t.Fatalf("err=%v calls=%d, want err=%v calls=%d", err, calls, tc.result, tc.wantCalls)
			}
		})
	}
	if store.IsSQLiteBusy(permanent) || store.IsSQLiteBusy(nil) {
		t.Fatal("non-SQLite error classified as busy")
	}
}

func TestRetrySQLiteBusyRecoversWhenTheWriterLockIsReleased(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)
	contender := openContender(t, db)
	holder := holdWriterLock(t, db)

	attempts := 0
	err := storage.RetrySQLiteBusy(ctx, func() error {
		attempts++
		if attempts == 2 {
			if err := holder.Commit(); err != nil {
				t.Errorf("release writer lock: %v", err)
			}
		}
		return beginAndRollback(ctx, contender)
	})

	if err != nil {
		t.Fatalf("retry did not recover from contention: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2: the first attempt hits the held writer lock, the second succeeds", attempts)
	}
}

func TestRetrySQLiteBusyStopsAtAttemptLimit(t *testing.T) {
	t.Parallel()
	db := openSingleConnectionDB(t)
	contender := openContender(t, db)
	holdWriterLock(t, db)
	busyErr := beginAndRollback(t.Context(), contender)
	if !store.IsSQLiteBusy(busyErr) {
		t.Fatalf("expected SQLite contention: %v", busyErr)
	}

	calls := 0
	err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return busyErr })

	if !errors.Is(err, busyErr) || calls != 6 {
		t.Fatalf("exhausted retry: err=%v calls=%d, want the busy error after 6 attempts", err, calls)
	}
}

func TestRetrySQLiteBusyStopsWhenTheContextIsCanceledDuringBackoff(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	contender := openContender(t, db)
	holdWriterLock(t, db)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	calls := 0
	err := storage.RetrySQLiteBusy(ctx, func() error {
		calls++
		return beginAndRollback(t.Context(), contender)
	})

	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled retry: err=%v attempts=%d, want context.Canceled after 1 attempt", err, calls)
	}
}

func TestRetrySQLiteBusyPreservesPermanentErrorOnCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	permanent := errors.New("permanent callback failure")
	calls := 0

	err := storage.RetrySQLiteBusy(ctx, func() error { calls++; return permanent })

	if !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("permanent error = %v, calls=%d", err, calls)
	}
}

func holdWriterLock(t *testing.T, db *storage.DB) *sql.Tx {
	t.Helper()
	holder, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback() })
	if _, err := holder.ExecContext(t.Context(), insertHeldEpoch); err != nil {
		t.Fatal(err)
	}
	return holder
}

func openContender(t *testing.T, db *storage.DB) *sql.DB {
	t.Helper()
	contender, err := sql.Open("sqlite", databasePath(t, db)+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contender.Close() })
	return contender
}

func beginAndRollback(ctx context.Context, contender *sql.DB) error {
	tx, err := contender.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	return tx.Rollback()
}

func databasePath(t *testing.T, db *storage.DB) string {
	t.Helper()
	var seq int
	var name, path string
	if err := db.QueryRowContext(t.Context(), "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}
