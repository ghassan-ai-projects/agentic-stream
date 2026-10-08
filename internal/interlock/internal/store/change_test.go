package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestChangeMovesTheInterlockOneVersionAtATime(t *testing.T) {
	t.Parallel()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "interlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var initial, tripped, cleared, read domain.State
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if initial, err = store.Read(t.Context(), tx); err != nil {
			return err
		}
		if tripped, err = store.Change(t.Context(), tx, "tripped", "operator stop", "2026-10-08T10:00:00Z"); err != nil {
			return err
		}
		if cleared, err = store.Change(t.Context(), tx, "ready", "inspected", "2026-10-08T10:05:00Z"); err != nil {
			return err
		}
		read, err = store.Read(t.Context(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if tripped.Version != initial.Version+1 || cleared.Version != initial.Version+2 || read != cleared || read.Status != "ready" || read.Reason != "inspected" {
		t.Fatalf("initial=%+v tripped=%+v cleared=%+v read=%+v", initial, tripped, cleared, read)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := store.Change(t.Context(), tx, "tripped", "", "2026-10-08T10:10:00Z")
		return err
	}); err == nil {
		t.Fatal("a change without a reason was accepted")
	}
}
