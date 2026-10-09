package storagetest_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestABackupIsAConsistentOpenableCopyAndNeverOverwrites(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := db.ExecContext(t.Context(), "INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at) VALUES ('c', 'k', 1, X'7B7D', 'now')"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "backup.db")
	if err := db.BackupInto(t.Context(), target); err != nil {
		t.Fatalf("BackupInto: %v", err)
	}
	copied, err := storage.OpenExisting(t.Context(), target)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = copied.Close() }()
	var rows int
	if err := copied.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM connector_checkpoints").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("backup rows = %d err=%v, want the source row", rows, err)
	}
	if err := db.BackupInto(t.Context(), target); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second backup over the same path = %v, want it refused", err)
	}
	if err := db.Vacuum(t.Context()); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
}
