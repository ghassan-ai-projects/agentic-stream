package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func copiedRunDatabase(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(watchRunDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "runtime.db")
	if err := os.WriteFile(path, source, 0o600); err != nil { //nolint:gosec // The path is the test's own temporary directory.
		t.Fatal(err)
	}
	return path
}

func TestPruneRemovesOnlyUnreferencedHistoryAndKeepsEverySituationReadable(t *testing.T) {
	t.Parallel()
	dbPath := copiedRunDatabase(t)
	situationID, _ := firstSituation(t, dbPath)
	var result pruneResult
	out := runCLI(t, newMaintenanceCommand(), "prune", "--db", dbPath, "--older-than", "0s", "--vacuum", "--json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("prune output %q: %v", out, err)
	}
	if result.InboxEntries == 0 {
		t.Fatalf("prune = %+v, want the applied inbox entries removed", result)
	}
	db, err := storage.OpenExisting(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var dangling int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&dangling); err != nil || dangling != 0 {
		t.Fatalf("foreign key violations after pruning = %d err=%v", dangling, err)
	}
	if show := runCLI(t, newSituationCommand(), "show", situationID, "--db", dbPath); !strings.Contains(show, situationID) {
		t.Fatalf("the current Situation is unreadable after pruning: %q", show)
	}
	if explain := runCLI(t, newExplainCommand(), "situation", situationID, "--db", dbPath); !strings.Contains(explain, "triggers:") {
		t.Fatalf("explain after pruning = %q", explain)
	}
}

func TestBackupWritesANewDatabaseFile(t *testing.T) {
	t.Parallel()
	dbPath := copiedRunDatabase(t)
	target := filepath.Join(t.TempDir(), "backup.db")
	if out := runCLI(t, newMaintenanceCommand(), "backup", "--db", dbPath, "--output", target); !strings.Contains(out, "backup="+target) {
		t.Fatalf("backup output = %q", out)
	}
	if info, err := os.Stat(target); err != nil || info.Size() == 0 {
		t.Fatalf("backup file = %v, %v", info, err)
	}
}
