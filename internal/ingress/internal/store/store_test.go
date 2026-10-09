package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func openStore(t *testing.T) (Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return New(db), db
}

func TestConfiguredNeedsTheDatabase(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	if !s.Configured() || New(nil).Configured() {
		t.Fatal("store configuration check changed")
	}
}

func TestCheckpointStartsAtZeroAndAdvancesPerConnector(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if line, err := s.LoadLine(t.Context(), "c1"); err != nil || line != 0 {
		t.Fatalf("fresh connector line = %d err=%v", line, err)
	}
	for _, line := range []int{3, 9} {
		if err := s.SaveLine(t.Context(), "c1", "jsonl-replay", line, now); err != nil {
			t.Fatal(err)
		}
	}
	if line, err := s.LoadLine(t.Context(), "c1"); err != nil || line != 9 {
		t.Fatalf("line = %d err=%v, want the latest", line, err)
	}
	if line, err := s.LoadLine(t.Context(), "c2"); err != nil || line != 0 {
		t.Fatalf("another connector must be independent: %d err=%v", line, err)
	}
}

func TestLoadRefusesStorageFailureAndCorruption(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at) VALUES ('bad', 'jsonl-replay', 1, X'7B', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadLine(t.Context(), "bad"); err == nil {
		t.Fatal("a corrupt checkpoint loaded")
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE connector_checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadLine(t.Context(), "c1"); err == nil {
		t.Fatal("a storage failure was treated as an empty checkpoint")
	}
}
