package storage_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestFacadeOpensRunsTransactionsAndCollectsRows(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "facade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "CREATE TABLE t (n INTEGER)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(t.Context(), "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	versions, err := storage.CollectRows(rows, "migrations", func(r *sql.Rows) (int, error) {
		var v int
		return v, r.Scan(&v)
	})
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions = %v err %v", versions, err)
	}
}

func TestFacadeRetriesOnlyBusyFailures(t *testing.T) {
	t.Parallel()
	if storage.IsSQLiteBusy(errors.New("plain")) {
		t.Fatal("a plain error is not busy")
	}
	calls := 0
	boom := errors.New("fatal")
	if err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return boom }); !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("non-busy error: calls=%d err=%v", calls, err)
	}
	if _, err := storage.OpenFresh(t.Context(), filepath.Join(t.TempDir(), "fresh.db")); err != nil {
		t.Fatalf("fresh database: %v", err)
	}
}

func TestFacadeNullIfEmptyAndQueryAll(t *testing.T) {
	t.Parallel()
	if storage.NullIfEmpty("").Valid || !storage.NullIfEmpty("x").Valid || storage.NullIfEmpty("x").String != "x" {
		t.Fatal("NullIfEmpty must be NULL only for the empty string")
	}
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "queryall.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	scan := func(rows *sql.Rows) (int, error) {
		var value int
		return value, rows.Scan(&value)
	}
	values, err := storage.QueryAll(t.Context(), db, "numbers", scan, "SELECT 1 UNION SELECT 2")
	if err != nil || len(values) != 2 {
		t.Fatalf("values = %v, %v", values, err)
	}
	none, err := storage.QueryAll(t.Context(), db, "numbers", scan, "SELECT 1 WHERE 0")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no rows = %v, %v; want an empty slice", none, err)
	}
	if _, err := storage.QueryAll(t.Context(), db, "numbers", scan, "SELECT * FROM missing_table"); err == nil {
		t.Fatal("a failing query was accepted")
	}
}
