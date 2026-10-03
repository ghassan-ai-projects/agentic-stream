package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRetrySQLiteBusyStopsAtAttemptLimit(t *testing.T) {
	t.Parallel()
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	path := dbPath(t, db)
	lock, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Rollback() })
	if _, err := lock.ExecContext(ctx, `INSERT INTO epoch_control VALUES ('held', 'draining', 'now')`); err != nil {
		t.Fatal(err)
	}
	contender, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contender.Close() })
	_, busyErr := contender.BeginTx(ctx, nil)
	if !storage.IsSQLiteBusy(busyErr) {
		t.Fatalf("expected SQLite contention: %v", busyErr)
	}
	calls := 0
	err = storage.RetrySQLiteBusy(ctx, func() error { calls++; return busyErr })
	if !errors.Is(err, busyErr) || calls != 6 {
		t.Fatalf("exhausted retry: err=%v calls=%d", err, calls)
	}
}

func TestRetrySQLiteBusyPreservesPermanentErrorOnCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// The callback is attempted before cancellation is consulted during backoff.
	permanent := errors.New("permanent callback failure")
	calls := 0
	err := storage.RetrySQLiteBusy(ctx, func() error { calls++; return permanent })
	if !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("permanent error = %v, calls=%d", err, calls)
	}
}

func dbPath(t *testing.T, db *storage.DB) string {
	t.Helper()
	var seq int
	var name, path string
	if err := db.QueryRowContext(t.Context(), "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}
