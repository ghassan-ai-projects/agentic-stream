package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

// newWatch builds a watch service whose ownership check always passes, for
// tests that exercise routing and pagination rather than runtime ownership.
func newWatch(t *testing.T, db *storage.DB) *watch.Service {
	t.Helper()
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	service, err := watch.New(watch.Config{DB: db, RuntimeOwner: owner, Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
