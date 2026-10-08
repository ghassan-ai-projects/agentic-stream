package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestCollectRowsScansInOrderAndPropagatesScanErrors(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "rows.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scanInt := func(rows *sql.Rows) (int, error) {
		var value int
		return value, rows.Scan(&value)
	}
	values, err := collect(t, db, "SELECT 1 UNION ALL SELECT 2 ORDER BY 1", scanInt)
	if err != nil || len(values) != 2 || values[0] != 1 || values[1] != 2 {
		t.Fatalf("CollectRows = %v, %v; want [1 2]", values, err)
	}
	if values, err := collect(t, db, "SELECT 1 WHERE 0", scanInt); err != nil || values == nil || len(values) != 0 {
		t.Fatalf("CollectRows on no rows = %v, %v; want an empty slice, nil", values, err)
	}
	failing := func(*sql.Rows) (int, error) { return 0, errors.New("scan failed") }
	if _, err := collect(t, db, "SELECT 1", failing); err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("CollectRows scan error = %v, want the scan error", err)
	}
}

func collect(t *testing.T, db *storage.DB, query string, scan func(*sql.Rows) (int, error)) ([]int, error) {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	return storage.CollectRows(rows, "values", scan)
}

func TestQueryAllScansEveryRowAndNamesWhatFailed(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "rows.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scanInt := func(rows *sql.Rows) (int, error) {
		var value int
		return value, rows.Scan(&value)
	}
	values, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT 1 UNION SELECT 2")
	if err != nil || len(values) != 2 {
		t.Fatalf("values = %v, %v", values, err)
	}
	empty, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT 1 WHERE 0")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("no rows = %v, %v; want an empty slice", empty, err)
	}
	if _, err := storage.QueryAll(t.Context(), db, "numbers", scanInt, "SELECT * FROM missing_table"); err == nil || !strings.Contains(err.Error(), "run query") {
		t.Fatalf("a failing query = %v", err)
	}
	failing := func(*sql.Rows) (int, error) { return 0, errors.New("scan failed") }
	if _, err := storage.QueryAll(t.Context(), db, "numbers", failing, "SELECT 1"); err == nil || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("a failing scan = %v", err)
	}
}

func TestQueryOptionalReportsAbsenceWithoutError(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "optional.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if value, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT 'x' UNION SELECT 'y' ORDER BY 1"); err != nil || !found || value != "x" {
		t.Fatalf("QueryOptional = %q, %v, %v; want the first row", value, found, err)
	}
	if value, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT 'x' WHERE 0"); err != nil || found || value != "" {
		t.Fatalf("QueryOptional on no rows = %q, %v, %v; want zero, false, nil", value, found, err)
	}
	if _, _, err := storage.QueryOptional[string](t.Context(), db, "SELECT * FROM missing_table"); err == nil || !strings.Contains(err.Error(), "run query") {
		t.Fatalf("QueryOptional on a bad query = %v, want run query error", err)
	}
	if _, _, err := storage.QueryOptional[int](t.Context(), db, "SELECT 'text'"); err == nil || !strings.Contains(err.Error(), "scan optional value") {
		t.Fatalf("QueryOptional on a mismatched type = %v, want scan error", err)
	}
}
