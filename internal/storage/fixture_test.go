package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func openOwnerDB(t *testing.T) (*storage.DB, time.Time) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return db, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
}
