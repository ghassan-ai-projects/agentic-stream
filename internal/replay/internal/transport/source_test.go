package transport

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestOpenSourceDatabaseIsReadOnly(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	seeded, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = seeded.Close()
	source, err := OpenSourceDatabase(t.Context(), path)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer func() { _ = source.Close() }()
	if _, err := source.DB.ExecContext(t.Context(), "CREATE TABLE written (id INTEGER)"); err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("a write to the source database = %v, want a readonly database error", err)
	}
}

func TestOpenSourceDatabaseRequiresAnExistingFile(t *testing.T) {
	t.Parallel()
	_, err := OpenSourceDatabase(t.Context(), filepath.Join(t.TempDir(), "missing.db"))
	if err == nil || !strings.Contains(err.Error(), "source database") {
		t.Fatalf("missing source opened: %v", err)
	}
	if err := (SourceDatabase{}).Close(); err != nil {
		t.Fatalf("closing an unopened source: %v", err)
	}
}
