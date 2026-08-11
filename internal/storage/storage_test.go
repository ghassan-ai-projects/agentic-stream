package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestOpenCreatesDatabaseAndRunsMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected migration version 1, got %d", version)
	}

	// Verify a known table exists.
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='event_log'").Scan(&name); err != nil {
		t.Fatalf("event_log table missing: %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	db1, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	stat, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}

	db2, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer func() { _ = db2.Close() }()

	// File should not have been recreated.
	stat2, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if stat2.ModTime().Before(stat.ModTime()) {
		t.Fatal("database was recreated on second open")
	}
}
