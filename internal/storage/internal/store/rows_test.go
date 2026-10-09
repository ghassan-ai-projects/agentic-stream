package store_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestCollectRowsScansInOrderAndPropagatesScanErrors(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	values, err := collect(t, db, "SELECT 1 UNION ALL SELECT 2 ORDER BY 1", scanOne[int])
	if err != nil || len(values) != 2 || values[0] != 1 || values[1] != 2 {
		t.Fatalf("CollectRows = %v, %v; want [1 2]", values, err)
	}
	if values, err := collect(t, db, "SELECT 1 WHERE 0", scanOne[int]); err != nil || values == nil || len(values) != 0 {
		t.Fatalf("CollectRows on no rows = %v, %v; want an empty slice, nil", values, err)
	}
	scanFailure := errors.New("scan failed")
	failing := func(*sql.Rows) (int, error) { return 0, scanFailure }
	if _, err := collect(t, db, "SELECT 1", failing); !errors.Is(err, scanFailure) {
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
