package actions_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	_ "modernc.org/sqlite"
)

func TestWatchEffectorIsBoundedExpiringAndOneShot(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	effector := actions.NewWatchEffector(db)
	command := actions.Command{
		CommandID: "cmd-watch", TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.temperature > 90", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z", "situation_id": "sit-1", "situation_version": 1, "max_fires": 1},
	}
	if _, err := effector.Dispatch(ctx, command); err != nil {
		t.Fatal(err)
	}
	firedCount, err := effector.FireEvent(ctx, "evt-1", "motor-1", map[string]any{"temperature": 95})
	if err != nil || firedCount != 1 {
		t.Fatalf("first fire count=%d err=%v", firedCount, err)
	}
	if _, err := effector.Dispatch(ctx, command); err != nil {
		t.Fatalf("idempotent reinstall after fire: %v", err)
	}
	fired, err := effector.Fire(ctx, "cmd-watch", "evt-2", "sit-1", "motor-1", map[string]any{"temperature": 95})
	if err != nil || fired {
		t.Fatalf("second fire=%v err=%v", fired, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM watch_conditions WHERE watch_id = ?", "cmd-watch").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" {
		t.Fatalf("watch status = %q, want disabled", status)
	}
}

func TestWatchEffectorEvaluatesExpressionAndExpiresWithoutAFire(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "watch-expiry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	virtual := clock.NewVirtual(now)
	effector := actions.NewWatchEffectorWithClock(db, virtual)
	command := actions.Command{
		CommandID: "cmd-expiry", TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.temperature > 90", "target": "motor-1", "expires_at": now.Add(time.Minute).Format(time.RFC3339Nano), "situation_id": "sit-1", "situation_version": 1, "max_fires": 2},
	}
	if _, err := effector.Dispatch(ctx, command); err != nil {
		t.Fatal(err)
	}
	fired, err := effector.Fire(ctx, "cmd-expiry", "evt-low", "sit-1", "motor-1", map[string]any{"temperature": 10})
	if err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Fatal("watch fired for a false expression")
	}
	virtual.Advance(2 * time.Minute)
	if err := effector.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	fired, err = effector.Fire(ctx, "cmd-expiry", "evt-late", "sit-1", "motor-1", map[string]any{"temperature": 100})
	if err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Fatal("expired watch fired")
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM watch_conditions WHERE watch_id = ?", "cmd-expiry").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Fatalf("watch status = %q, want expired", status)
	}
}

func TestWatchEffectorExpireRetriesAfterSQLiteBusy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "watch-contended.db")
	db, err := storage.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	virtual := clock.NewVirtual(now)
	effector := actions.NewWatchEffectorWithClock(db, virtual)
	command := actions.Command{
		CommandID: "cmd-contended", TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression": "features.temperature > 90", "target": "motor-1",
			"expires_at": now.Add(time.Minute).Format(time.RFC3339Nano), "situation_id": "sit-1",
			"situation_version": 1, "max_fires": 1,
		},
	}
	if _, err := effector.Dispatch(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	virtual.Advance(2 * time.Minute)

	// Make the runtime connection fail fast so this test exercises the
	// application retry rather than waiting for the production timeout.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA busy_timeout = 1"); err != nil {
		t.Fatal(err)
	}
	var busyTimeout int
	if err := db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 1 {
		t.Fatalf("test busy timeout = %d ms, want 1 ms", busyTimeout)
	}

	raw, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	locker, err := raw.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locker.Close() }()
	if _, err := locker.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if _, err := locker.ExecContext(t.Context(), "UPDATE watch_conditions SET updated_at = updated_at WHERE watch_id = 'cmd-contended'"); err != nil {
		t.Fatal(err)
	}

	released := make(chan error, 1)
	go func() {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		<-timer.C
		_, rollbackErr := locker.ExecContext(context.Background(), "ROLLBACK")
		released <- rollbackErr
	}()

	started := time.Now()
	err = effector.Expire(t.Context())
	elapsed := time.Since(started)
	if releaseErr := <-released; releaseErr != nil {
		t.Fatalf("release SQLite lock: %v", releaseErr)
	}
	if err != nil {
		t.Fatalf("Expire under a transient SQLite lock: %v", err)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("Expire completed in %s; expected a busy retry after the lock was released", elapsed)
	}

	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM watch_conditions WHERE watch_id = ?", "cmd-contended").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Fatalf("watch status = %q, want expired", status)
	}
}
