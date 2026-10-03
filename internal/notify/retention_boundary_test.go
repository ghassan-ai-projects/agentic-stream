package notify_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPruningPreservesHighwaterAndRejectsExpiredEvents(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	original := testEvent("original", now)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { _, err := notify.Append(t.Context(), tx, original, now); return err }); err != nil {
		t.Fatal(err)
	}
	later := now.Add(8 * 24 * time.Hour)
	if deleted, err := notify.Prune(t.Context(), db, later, 7*24*time.Hour); err != nil || deleted != 1 {
		t.Fatalf("prune = %d, %v", deleted, err)
	}
	if _, err := notify.ReadPage(t.Context(), db, "tenant", 0, 10, 0, later); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("old cursor = %v", err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { _, err := notify.Append(t.Context(), tx, original, later); return err }); !errors.Is(err, notify.ErrEventExpired) {
		t.Fatalf("expired event = %v", err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		cursor, err := notify.Append(t.Context(), tx, testEvent("next", later), later)
		if err == nil && cursor != 2 {
			t.Errorf("next cursor = %d, want 2", cursor)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
