package store_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"

	_ "modernc.org/sqlite"
)

var runtimePragmas = []struct{ pragma, want string }{
	{"busy_timeout", "30000"},
	{"journal_mode", "wal"},
	{"synchronous", "1"},
	{"wal_autocheckpoint", "256"},
	{"foreign_keys", "1"},
}

func TestOpenCreatesTheDatabaseWithEveryMigrationAndTheRuntimePragmas(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database exists before Open: %v", err)
	}

	db, err := storage.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	all, err := migrations.All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if got := latestMigration(t, db); got != all[len(all)-1].Version {
		t.Fatalf("latest applied migration = %d, want %d", got, all[len(all)-1].Version)
	}
	for _, pragma := range runtimePragmas {
		if got := pragmaValue(t, db, pragma.pragma); got != pragma.want {
			t.Errorf("PRAGMA %s = %s, want %s", pragma.pragma, got, pragma.want)
		}
	}
}

func TestEveryPooledConnectionKeepsTheRuntimePragmas(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	db.SetMaxOpenConns(2)
	first, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("acquire first pooled connection: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("acquire second pooled connection: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	for name, conn := range map[string]*sql.Conn{"first": first, "second": second} {
		for _, pragma := range runtimePragmas {
			var got string
			if err := conn.QueryRowContext(t.Context(), "PRAGMA "+pragma.pragma).Scan(&got); err != nil {
				t.Fatalf("%s connection PRAGMA %s: %v", name, pragma.pragma, err)
			}
			if got != pragma.want {
				t.Errorf("%s connection PRAGMA %s = %s, want %s", name, pragma.pragma, got, pragma.want)
			}
		}
	}
}

func TestOpeningAMigratedDatabaseAgainAppliesNothing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	first, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	before := appliedMigrations(t, first)
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	second, err := storage.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if after := appliedMigrations(t, second); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("reopening changed the migration ledger:\nbefore %v\nafter  %v", before, after)
	}
}

func TestAFailedMigrationRollsBackAndNamesTheMigration(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conflict.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, raw, "CREATE TABLE artifacts (blocker TEXT)")

	db, err := storage.Open(t.Context(), path)

	if err == nil || !strings.Contains(err.Error(), "migration 1 initial: execute") {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("Open = %v, want a failure naming migration 1 initial", err)
	}
	tables, err := storage.QueryAll(t.Context(), raw, "tables", scanOne[string],
		"SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	_ = raw.Close()
	if err != nil || fmt.Sprint(tables) != "[artifacts]" {
		t.Fatalf("tables after the failed migration = %v, %v; want only the conflicting table", tables, err)
	}
}

func TestOpenReportsAReservationItCannotInspect(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := storage.Open(t.Context(), filepath.Join(file, "runtime.db"))

	if err == nil || !strings.Contains(err.Error(), "inspect replay reservation") {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("Open = %v, want an inspect replay reservation failure", err)
	}
}

func latestMigration(t *testing.T, db *storage.DB) int {
	t.Helper()
	version, found, err := storage.QueryOptional[int](t.Context(), db, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1")
	if err != nil || !found {
		t.Fatalf("read latest migration: found=%v err=%v", found, err)
	}
	return version
}

func appliedMigrations(t *testing.T, db *storage.DB) []string {
	t.Helper()
	applied, err := storage.QueryAll(t.Context(), db, "applied migrations", scanMigrationRecord,
		"SELECT version, name, applied_at FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	return applied
}

func pragmaValue(t *testing.T, db *storage.DB, pragma string) string {
	t.Helper()
	value, found, err := storage.QueryOptional[string](t.Context(), db, "PRAGMA "+pragma)
	if err != nil || !found {
		t.Fatalf("PRAGMA %s: found=%v err=%v", pragma, found, err)
	}
	return value
}

func scanMigrationRecord(rows *sql.Rows) (string, error) {
	var version int
	var name, appliedAt string
	if err := rows.Scan(&version, &name, &appliedAt); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d/%s/%s", version, name, appliedAt), nil
}
