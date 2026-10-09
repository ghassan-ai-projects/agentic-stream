package app_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAnExpiredWatchIsMarkedExpiredKeptForAuditAndNeverFires(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	clock := sources.NewVirtual(fixtureNow)
	service := newServiceAt(t, db, clock)
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 2, fixtureNow.Add(time.Minute)))
	if fired, err := service.FireEvent(t.Context(), "evt-low", "motor-1", map[string]any{"temperature": 10}); err != nil || fired != 0 {
		t.Fatalf("a false expression fired %d, %v", fired, err)
	}

	clock.Advance(2 * time.Minute)
	if err := service.Expire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fired, err := service.FireEvent(t.Context(), "evt-late", "motor-1", map[string]any{"temperature": 100}); err != nil || fired != 0 {
		t.Fatalf("an expired watch fired %d, %v", fired, err)
	}
	if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "expired", Remaining: 2}); got != want {
		t.Fatalf("watch = %+v, want %+v: expired, its allowance and audit row kept", got, want)
	}
}

func TestExpiringLeavesActiveAndSpentWatchesAlone(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	clock := sources.NewVirtual(fixtureNow)
	service := newServiceAt(t, db, clock)
	install(t, service, watchCommand("cmd-active", "features.t > 1", "motor-1", 1, fixtureNow.Add(time.Hour)))
	install(t, service, watchCommand("cmd-spent", "features.t > 1", "motor-2", 1, fixtureNow.Add(time.Minute)))
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-2", map[string]any{"t": 5}); err != nil || fired != 1 {
		t.Fatalf("fired = %d, %v", fired, err)
	}
	clock.Advance(2 * time.Minute)
	if err := service.Expire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if active, spent := readWatch(t, db, "cmd-active").Status, readWatch(t, db, "cmd-spent").Status; active != "active" || spent != "disabled" {
		t.Fatalf("active = %q, spent = %q; want active and disabled", active, spent)
	}
}

func installExpiredWatchOnDisk(t *testing.T, clock interface{ Advance(time.Duration) }, source sources.Clock) (string, *storage.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "watch-contended.db")
	db, err := storagetest.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	install(t, newServiceAt(t, db, source), watchCommand("cmd-contended", "features.temperature > 90", "motor-1", 1, fixtureNow.Add(time.Minute)))
	clock.Advance(2 * time.Minute)
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA busy_timeout = 1"); err != nil {
		t.Fatal(err)
	}
	return dbPath, db
}

func holdWriteLock(t *testing.T, dbPath string) *sql.Conn {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	locker, err := raw.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = locker.Close() })
	for _, statement := range []string{"BEGIN IMMEDIATE", "UPDATE watch_conditions SET updated_at = updated_at WHERE watch_id = 'cmd-contended'"} {
		if _, err := locker.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return locker
}

func TestExpiringRetriesWhileTheDatabaseIsBusyAndSucceedsOnceItIsReleased(t *testing.T) {
	t.Parallel()
	clock := sources.NewVirtual(fixtureNow)
	dbPath, db := installExpiredWatchOnDisk(t, clock, clock)
	locker := holdWriteLock(t, dbPath)
	var released atomic.Bool
	releaseErr := make(chan error, 1)
	timer := time.AfterFunc(50*time.Millisecond, func() {
		released.Store(true)
		_, err := locker.ExecContext(context.Background(), "ROLLBACK")
		releaseErr <- err
	})
	defer timer.Stop()

	err := newServiceAt(t, db, clock).Expire(t.Context())
	if err != nil {
		t.Fatalf("Expire under a transient SQLite lock: %v", err)
	}
	if !released.Load() {
		t.Fatal("Expire succeeded while the lock was still held")
	}
	if err := <-releaseErr; err != nil {
		t.Fatalf("release the SQLite lock: %v", err)
	}
	if got := readWatch(t, db, "cmd-contended").Status; got != "expired" {
		t.Fatalf("watch status = %q, want expired", got)
	}
}

func TestExpiringHonorsCancellationWhileTheDatabaseStaysBusy(t *testing.T) {
	t.Parallel()
	clock := sources.NewVirtual(fixtureNow)
	dbPath, db := installExpiredWatchOnDisk(t, clock, clock)
	holdWriteLock(t, dbPath)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := newServiceAt(t, db, clock).Expire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Expire under a held SQLite lock = %v, want the context deadline", err)
	}
}
