package storagetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Open opens the runtime database at path exactly as storage.Open does. When
// no database exists at path yet, it first seeds the file with the migrated
// template so the open finds every migration applied.
func Open(ctx context.Context, path string) (*storage.DB, error) {
	if err := seedFromTemplate(path); err != nil {
		return nil, err
	}
	db, err := storage.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open seeded database: %w", err)
	}
	return db, nil
}

// OpenTemp opens a migrated runtime database in the test's temporary
// directory and closes it when the test ends. It fails the test on error.
func OpenTemp(tb testing.TB) *storage.DB {
	tb.Helper()
	db, err := Open(tb.Context(), filepath.Join(tb.TempDir(), "runtime.db"))
	if err != nil {
		tb.Fatalf("open runtime database: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

// OpenTempWithoutForeignKeys is OpenTemp for tests that seed rows without
// their parents: one connection, foreign key enforcement off.
func OpenTempWithoutForeignKeys(tb testing.TB) *storage.DB {
	tb.Helper()
	db := OpenTemp(tb)
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(tb.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		tb.Fatalf("disable foreign keys: %v", err)
	}
	return db
}

func seedFromTemplate(path string) error {
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	template, err := migratedTemplate()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, template, 0o600); err != nil {
		return fmt.Errorf("seed database %s: %w", path, err)
	}
	return nil
}
