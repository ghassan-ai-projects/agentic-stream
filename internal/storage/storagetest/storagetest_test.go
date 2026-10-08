package storagetest_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

func countApplied(t *testing.T, db *storage.DB) int {
	t.Helper()
	count, found, err := storage.QueryOptional[int](t.Context(), db, "SELECT COUNT(*) FROM schema_migrations")
	if err != nil || !found {
		t.Fatalf("count applied migrations: found=%v err=%v", found, err)
	}
	return count
}

func TestOpenAppliesEveryMigration(t *testing.T) {
	t.Parallel()
	all, err := migrations.All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if got := countApplied(t, db); got != len(all) {
		t.Fatalf("applied migrations = %d, want %d", got, len(all))
	}
}

func TestOpenGivesEachPathItsOwnDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, err := storagetest.Open(t.Context(), filepath.Join(dir, "first.db"))
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := storagetest.Open(t.Context(), filepath.Join(dir, "second.db"))
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if _, err := first.ExecContext(t.Context(), "DELETE FROM schema_migrations"); err != nil {
		t.Fatalf("clear first: %v", err)
	}
	if got := countApplied(t, second); got == 0 {
		t.Fatal("second database lost its migrations when the first was changed")
	}
}

func TestOpenKeepsAnExistingDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := storagetest.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE kept (id INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, found, err := storage.QueryOptional[string](t.Context(), reopened, "SELECT name FROM sqlite_master WHERE name = 'kept'"); err != nil || !found {
		t.Fatalf("existing table lost on reopen: found=%v err=%v", found, err)
	}
}

func TestOpenReportsAnUnwritableSeedPath(t *testing.T) {
	t.Parallel()
	missingDirectory := filepath.Join(t.TempDir(), "missing", "runtime.db")
	if db, err := storagetest.Open(t.Context(), missingDirectory); err == nil {
		_ = db.Close()
		t.Fatal("open succeeded under a directory that does not exist")
	}
}

func TestTemplateIsBuiltOnceAndReusedFromItsDirectory(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "templates")
	built, err := storagetest.LoadTemplate(directory)
	if err != nil {
		t.Fatalf("build template: %v", err)
	}
	reused, err := storagetest.LoadTemplate(directory)
	if err != nil {
		t.Fatalf("reuse template: %v", err)
	}
	if len(built) == 0 || !bytes.Equal(built, reused) {
		t.Fatalf("template changed between builds: %d bytes then %d bytes", len(built), len(reused))
	}
}

func TestOpenTempGivesAMigratedDatabaseThatClosesWithTheTest(t *testing.T) {
	t.Parallel()
	var db *storage.DB
	t.Run("session", func(t *testing.T) {
		db = storagetest.OpenTemp(t)
		if applied := countApplied(t, db); applied == 0 {
			t.Fatal("OpenTemp returned a database with no migrations applied")
		}
	})
	if err := db.PingContext(t.Context()); err == nil {
		t.Fatal("OpenTemp left the database open after its test finished")
	}
}

func TestOpenTempWithoutForeignKeysAllowsOrphanRows(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if _, err := db.ExecContext(t.Context(), "INSERT INTO situation_versions (situation_id, version) VALUES ('missing', 1)"); err != nil && strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("foreign keys are still enforced: %v", err)
	}
	found, _, err := storage.QueryOptional[int](t.Context(), db, "PRAGMA foreign_keys")
	if err != nil || found != 0 {
		t.Fatalf("PRAGMA foreign_keys = %d, %v; want 0", found, err)
	}
}
