package store_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func openSingleConnectionDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)
	db.SetMaxOpenConns(1)
	return db
}
