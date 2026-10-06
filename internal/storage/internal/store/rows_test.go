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
	if values, err := collect(t, db, "SELECT 1 WHERE 0", scanInt); err != nil || values != nil {
		t.Fatalf("CollectRows on no rows = %v, %v; want nil, nil", values, err)
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
