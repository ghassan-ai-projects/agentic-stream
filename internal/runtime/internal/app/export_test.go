package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func NewWatch(t *testing.T, db *storage.DB) *watch.Service {
	t.Helper()
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	service, err := watch.New(watch.Config{DB: db, RuntimeOwner: owner, Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
