package storage_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestFacadeDelegatesToTheStore(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	fresh, err := storage.OpenFresh(ctx, filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("OpenFresh: %v", err)
	}
	if err := fresh.Close(); err != nil {
		t.Fatalf("close fresh database: %v", err)
	}
	calls := 0
	if err := storage.RetrySQLiteBusy(ctx, func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("RetrySQLiteBusy = %v after %d calls, want nil after 1", err, calls)
	}
	if storage.IsSQLiteBusy(errors.New("plain")) {
		t.Fatal("a plain error is not busy")
	}
	_, duplicate := db.ExecContext(ctx, "INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, 'again', 'now')")
	if !storage.IsUniqueViolation(duplicate, "schema_migrations.version") {
		t.Fatalf("IsUniqueViolation(%v) = false for a duplicate primary key", duplicate)
	}
	name, found, err := storage.QueryOptional[string](ctx, db, "SELECT name FROM schema_migrations WHERE version = 1")
	if err != nil || !found || name != "initial" {
		t.Fatalf("QueryOptional = %q, %v, %v; want the first migration", name, found, err)
	}
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version LIMIT 2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	versions, err := storage.CollectRows(rows, "migrations", scanInt)
	if err != nil || len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
		t.Fatalf("CollectRows = %v, %v; want [1 2]", versions, err)
	}
}

func TestNullIfEmptyIsNullOnlyForTheEmptyString(t *testing.T) {
	t.Parallel()
	if got := storage.NullIfEmpty(""); got.Valid {
		t.Fatalf("NullIfEmpty(\"\") = %+v, want NULL", got)
	}
	if got := storage.NullIfEmpty("x"); !got.Valid || got.String != "x" {
		t.Fatalf("NullIfEmpty(\"x\") = %+v, want the string", got)
	}
}

func TestQueryAllScansEveryRowAndNamesWhatFailed(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	values, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT 1 UNION SELECT 2")
	if err != nil || len(values) != 2 || values[0] != 1 || values[1] != 2 {
		t.Fatalf("values = %v, %v; want [1 2]", values, err)
	}
	empty, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT 1 WHERE 0")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("no rows = %v, %v; want an empty slice", empty, err)
	}
	if _, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT * FROM missing_table"); err == nil || !strings.Contains(err.Error(), "run query") {
		t.Fatalf("a failing query = %v, want a run query error", err)
	}
	scanFailure := errors.New("scan failed")
	failing := func(*sql.Rows) (int, error) { return 0, scanFailure }
	if _, err := storage.QueryAll(t.Context(), db, "numbers", failing, "SELECT 1"); !errors.Is(err, scanFailure) {
		t.Fatalf("a failing scan = %v, want the scan error", err)
	}
}

func TestInClauseBindsOnePlaceholderPerValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		values []string
		clause string
	}{
		{"one", []string{"a"}, "c IN (?)"},
		{"many", []string{"a", "b", "c"}, "c IN (?, ?, ?)"},
		{"none", nil, "c IN ()"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clause, args := storage.InClause("c", tc.values)
			if clause != tc.clause || len(args) != len(tc.values) {
				t.Fatalf("clause = %q args = %v, want %q", clause, args, tc.clause)
			}
			for i, value := range tc.values {
				if args[i] != value {
					t.Fatalf("arg %d = %v, want %q", i, args[i], value)
				}
			}
		})
	}
}

func TestRowsAffectedCountsChangedRowsAndReportsZeroWhenTheDriverCannot(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE counted (n INTEGER)"); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(t.Context(), "INSERT INTO counted (n) VALUES (1), (2), (3)")
	if err != nil {
		t.Fatal(err)
	}
	if got := storage.RowsAffected(result); got != 3 {
		t.Fatalf("RowsAffected = %d, want 3", got)
	}
	if got := storage.RowsAffected(unreportedResult{}); got != 0 {
		t.Fatalf("RowsAffected without driver support = %d, want 0", got)
	}
}

func TestBoolIntIsOneForTrueAndZeroForFalse(t *testing.T) {
	t.Parallel()
	if storage.BoolInt(true) != 1 || storage.BoolInt(false) != 0 {
		t.Fatalf("BoolInt(true)=%d BoolInt(false)=%d, want 1 and 0", storage.BoolInt(true), storage.BoolInt(false))
	}
}

type unreportedResult struct{}

func (unreportedResult) LastInsertId() (int64, error) { return 0, errors.New("unsupported") }
func (unreportedResult) RowsAffected() (int64, error) { return 0, errors.New("unsupported") }

func scanInt(rows *sql.Rows) (int, error) {
	var value int
	err := rows.Scan(&value)
	return value, err
}
