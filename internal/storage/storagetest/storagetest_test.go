package storagetest_test

import (
	"bytes"
	"context"
	"os"
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

func TestTemplateIsBuiltOnceReusedAndOlderTemplatesAreDeleted(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	older := filepath.Join(directory, "template-0123456789abcdef.db")
	unrelated := filepath.Join(directory, "notes.txt")
	for _, planted := range []string{older, unrelated} {
		if err := os.WriteFile(planted, []byte("planted"), 0o600); err != nil {
			t.Fatalf("plant %s: %v", planted, err)
		}
	}
	built, err := storagetest.LoadTemplate(directory)
	if err != nil {
		t.Fatalf("build template: %v", err)
	}
	current, err := storagetest.TemplatePath(directory)
	if err != nil {
		t.Fatalf("template path: %v", err)
	}
	builtFile := statFile(t, current)

	reused, err := storagetest.LoadTemplate(directory)
	if err != nil {
		t.Fatalf("reuse template: %v", err)
	}

	if len(built) == 0 || !bytes.Equal(built, reused) {
		t.Fatalf("template changed between loads: %d bytes then %d bytes", len(built), len(reused))
	}
	if reusedFile := statFile(t, current); !os.SameFile(builtFile, reusedFile) || !reusedFile.ModTime().Equal(builtFile.ModTime()) {
		t.Fatal("the second load published a new template file instead of reusing the first")
	}
	for planted, wantKept := range map[string]bool{older: false, unrelated: true, current: true} {
		if _, err := os.Lstat(planted); (err == nil) != wantKept {
			t.Errorf("%s kept = %t, want %t", filepath.Base(planted), err == nil, wantKept)
		}
	}
}

func statFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info
}

func TestOpenTempGivesAMigratedDatabaseThatClosesWithTheTest(t *testing.T) {
	t.Parallel()
	var db *storage.DB
	t.Cleanup(func() {
		if err := db.PingContext(context.WithoutCancel(t.Context())); err == nil {
			t.Error("OpenTemp left the database open after its test finished")
		}
	})
	db = storagetest.OpenTemp(t)
	if applied := countApplied(t, db); applied == 0 {
		t.Fatal("OpenTemp returned a database with no migrations applied")
	}
}

func TestOpenTempWithoutForeignKeysAllowsOrphanRows(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	for _, statement := range []string{
		"CREATE TABLE parents (id INTEGER PRIMARY KEY)",
		"CREATE TABLE children (parent_id INTEGER REFERENCES parents(id))",
		"INSERT INTO children (parent_id) VALUES (42)",
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("max open connections = %d, want 1", got)
	}
}

func TestOpenTempEnforcesForeignKeys(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	for _, statement := range []string{
		"CREATE TABLE parents (id INTEGER PRIMARY KEY)",
		"CREATE TABLE children (parent_id INTEGER REFERENCES parents(id))",
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	_, err := db.ExecContext(t.Context(), "INSERT INTO children (parent_id) VALUES (42)")
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("orphan insert = %v, want a FOREIGN KEY failure", err)
	}
}

func TestTemplateIsRebuiltWhenTheCachedFileIsUnsound(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path, err := storagetest.TemplatePath(directory)
	if err != nil {
		t.Fatalf("template path: %v", err)
	}
	planted := []byte("this is not a sqlite database, but it is long enough to be read as one")
	if err := os.WriteFile(path, planted, 0o600); err != nil {
		t.Fatalf("plant cached file: %v", err)
	}

	rebuilt, err := storagetest.LoadTemplate(directory)

	if err != nil || len(rebuilt) == 0 || bytes.Equal(rebuilt, planted) {
		t.Fatalf("unsound cache was not rebuilt: %d bytes, err=%v", len(rebuilt), err)
	}
	if onDisk, err := os.ReadFile(path); err != nil || !bytes.Equal(onDisk, rebuilt) {
		t.Fatalf("rebuilt template was not published: err=%v", err)
	}
}

func TestAnUnsoundCachedTemplateIsRefused(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content []byte
	}{
		{"empty file", []byte{}},
		{"not a database", []byte("this is not a sqlite database, but it is long enough to be read as one")},
		{"truncated header", []byte("SQLite format 3\x00")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "template.db")
			if err := os.WriteFile(path, test.content, 0o600); err != nil {
				t.Fatalf("plant cached file: %v", err)
			}
			if content, err := storagetest.ReadSoundTemplate(path); err == nil {
				t.Fatalf("ReadSoundTemplate accepted %s (%d bytes)", test.name, len(content))
			}
		})
	}
}
