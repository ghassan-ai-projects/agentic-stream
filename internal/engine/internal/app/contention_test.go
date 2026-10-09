package app_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// TestRecordIsRetriedAfterTransientSQLiteBusy makes the first apply attempt
// fail with a real SQLITE_BUSY: inside the engine's transaction, which holds the
// write lock, the owner check asks a second connection for the same lock.
func TestRecordIsRetriedAfterTransientSQLiteBusy(t *testing.T) {
	t.Parallel()
	path, db := openDatabaseFile(t)
	contender := openContender(t, path)
	attempts := 0
	owner := func(ctx context.Context, _ *sql.Tx, _ string) error {
		attempts++
		if attempts > 1 {
			return nil
		}
		_, err := contender.ExecContext(ctx, "BEGIN IMMEDIATE")
		if err == nil {
			t.Error("the second connection acquired the write lock held by the engine's transaction")
			_, _ = contender.ExecContext(ctx, "ROLLBACK")
		}
		return err
	}
	compiled := restartSpec()
	log := eventlog.NewEventLog(db)
	service, err := newServiceWithOwner(t.Context(), db, log, sources.Physical(), &compiled, "default", false, owner)
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, log, "evt-contention", 0, 15)

	if processed := runGlobal(t, service); processed != 1 {
		t.Fatalf("processed %d events, want 1", processed)
	}
	if attempts != 2 {
		t.Fatalf("apply attempts = %d, want 2: one busy failure then one success", attempts)
	}
	if applied := countRows(t, db, "SELECT COUNT(*) FROM event_inbox"); applied != 1 {
		t.Fatalf("inbox rows = %d, want the event applied once", applied)
	}
}

func openContender(t *testing.T, path string) *sql.Conn {
	t.Helper()
	contenderDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contenderDB.Close() })
	contender, err := contenderDB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contender.Close() })
	return contender
}
