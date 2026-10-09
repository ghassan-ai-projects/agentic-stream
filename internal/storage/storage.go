package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

// IsUniqueViolation reports whether err is a SQLite uniqueness failure (primary
// key or unique index) on column, written as table.column.
func IsUniqueViolation(err error, column string) bool {
	return store.IsUniqueViolation(err, column)
}

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

// NullIfEmpty is the SQL NULL for an empty string and the string otherwise.
func NullIfEmpty(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// InClause is the SQL condition `column IN (?, ?, ...)` for values and the
// arguments that bind its placeholders, in order. The column is spliced into
// the statement as text, so it must be a literal identifier of the caller's
// own SQL, never data; only the values are bound.
func InClause(column string, values []string) (string, []any) {
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = value
	}
	return column + " IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(values)), ", ") + ")", args
}

// RowsAffected is the number of rows a statement changed, or zero when the
// driver cannot report it.
func RowsAffected(result sql.Result) int64 {
	count, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return count
}

// BoolInt is the SQL integer of a boolean: 1 for true, 0 for false.
func BoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Querier is anything that can run a query: *sql.DB, *sql.Tx and *DB.
type Querier = store.Querier

// OwnerCheck is a write fence: it runs on the caller's transaction and fails
// when the given epoch no longer owns the runtime. Control's owner assertion
// satisfies it; every module that fences its writes takes this one type.
type OwnerCheck = store.OwnerCheck

// QueryOptional scans the first column of the first row of query. A query
// without rows reports found as false and no error; callers add the operation.
func QueryOptional[T any](ctx context.Context, q Querier, query string, args ...any) (value T, found bool, err error) {
	return store.QueryOptional[T](ctx, q, query, args...) //nolint:wrapcheck // Single delegating facade return; the store reports the failed query and scan.
}

// QueryAll runs query, scans every row with scan and closes the rows. A query
// failure is "run query: <cause>" and an iteration failure names what was
// being read; callers add the operation. No rows yields an empty, non-nil slice.
func QueryAll[T any](ctx context.Context, q Querier, what string, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("run query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return CollectRows(rows, what, scan)
}
