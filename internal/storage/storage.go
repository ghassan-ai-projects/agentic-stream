package storage

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/store"
)

// DB wraps a sql.DB with runtime-specific configuration: WAL, foreign keys, a
// busy timeout and immediate write transactions.
type DB = store.DB

// Open opens or creates the SQLite database at path and runs pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	return store.Open(ctx, path) //nolint:wrapcheck // The store names the failed step.
}

// OpenFresh atomically reserves a new database path before opening SQLite, for
// isolated replay.
func OpenFresh(ctx context.Context, path string) (*DB, error) {
	return store.OpenFresh(ctx, path) //nolint:wrapcheck // The store names the failed step.
}

// IsSQLiteBusy reports whether err is a retryable SQLite busy or locked result.
func IsSQLiteBusy(err error) bool { return store.IsSQLiteBusy(err) }

// RetrySQLiteBusy retries fn after transient SQLite writer contention. The
// callback must roll back its transaction before it returns.
func RetrySQLiteBusy(ctx context.Context, fn func() error) error {
	return store.RetrySQLiteBusy(ctx, fn) //nolint:wrapcheck // The store names the failed step.
}

// CollectRows scans every remaining row with scan, in order. The caller owns
// and closes rows.
func CollectRows[T any](rows *sql.Rows, what string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	return store.CollectRows(rows, what, scan) //nolint:wrapcheck // Scan errors are returned as is.
}
