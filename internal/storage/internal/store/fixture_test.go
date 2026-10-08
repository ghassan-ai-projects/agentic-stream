package store_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func openOwnerDB(t *testing.T) (*storage.DB, time.Time) {
	t.Helper()
	db := storagetest.OpenTemp(t)

	db.SetMaxOpenConns(1)
	return db, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
}
