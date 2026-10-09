package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/replaytest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func seededReplayContext(t *testing.T) context.Context {
	t.Helper()
	return replaytest.WithDatabaseOpener(t.Context(), storagetest.Open)
}

const pollEvery = 25 * time.Millisecond

func pollUntil(t *testing.T, timeout time.Duration, ready func() bool) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		if ready() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func newMigratedDatabasePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.db")
	seedMigratedDatabase(t, path)
	return path
}

func seedMigratedDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("seed migrated database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded database: %v", err)
	}
}

func countRows(t *testing.T, dbPath, table string) int {
	t.Helper()
	var count int
	if err := openReadOnly(t, dbPath).QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil { //nolint:gosec // table names are literals in the tests
		t.Fatal(err)
	}
	return count
}
