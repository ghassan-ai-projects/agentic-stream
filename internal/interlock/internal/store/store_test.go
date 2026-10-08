package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAssertFailsClosedAndVersionIsMonotonic(t *testing.T) {
	db, err := storagetest.Open(context.Background(), filepath.Join(t.TempDir(), "interlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.Assert(context.Background(), tx)
	}); err != nil {
		t.Fatalf("initial interlock: %v", err)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.Set(context.Background(), tx, "tripped", "test stop", 2, now)
	}); err != nil {
		t.Fatalf("trip interlock: %v", err)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		if err := store.Assert(context.Background(), tx); err == nil {
			return fmt.Errorf("expected tripped interlock rejection")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		if err := store.Set(context.Background(), tx, "ready", "stale reopen", 1, now); err == nil {
			return fmt.Errorf("expected stale interlock update rejection")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
