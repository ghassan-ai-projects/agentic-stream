package interlock_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func allow(t *testing.T) interlock.Fence {
	t.Helper()
	return func(context.Context, *sql.Tx) error { return nil }
}

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "interlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func admits(t *testing.T, db *storage.DB) error {
	t.Helper()
	return db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return interlock.Assert(t.Context(), tx)
	})
}

func TestTripBlocksAndClearReopensTheActionPlane(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if err := admits(t, db); err != nil {
		t.Fatalf("the seeded interlock must be ready: %v", err)
	}
	tripped, err := interlock.Trip(t.Context(), db, "operator stop", now)
	if err != nil || tripped.Status != "tripped" || tripped.Version != 2 || tripped.Reason != "operator stop" {
		t.Fatalf("trip = %+v, %v", tripped, err)
	}
	if err := admits(t, db); !errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("a tripped interlock must refuse: %v", err)
	}
	cleared, err := interlock.Clear(t.Context(), db, allow(t), "inspected", now)
	if err != nil || cleared.Status != "ready" || cleared.Version != 3 {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	if err := admits(t, db); err != nil {
		t.Fatalf("a cleared interlock must admit: %v", err)
	}
	status, err := interlock.Status(t.Context(), db)
	if err != nil || status != cleared {
		t.Fatalf("status = %+v, %v; want %+v", status, err, cleared)
	}
}

func TestClearIsRefusedWithoutTheFenceAndWritesNothing(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if _, err := interlock.Trip(t.Context(), db, "operator stop", now); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("not the owner")
	fence := func(context.Context, *sql.Tx) error { return denied }
	if _, err := interlock.Clear(t.Context(), db, fence, "too early", now); !errors.Is(err, denied) {
		t.Fatalf("a clear the fence refused = %v", err)
	}
	if _, err := interlock.Clear(t.Context(), db, nil, "no fence", now); err == nil {
		t.Fatal("a clear without a fence was accepted")
	}
	status, err := interlock.Status(t.Context(), db)
	if err != nil || status.Status != "tripped" || status.Version != 2 {
		t.Fatalf("a refused clear changed the interlock: %+v, %v", status, err)
	}
}

func TestChangesNeedAReason(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if _, err := interlock.Trip(t.Context(), db, "", now); err == nil {
		t.Fatal("a trip without a reason was accepted")
	}
}
