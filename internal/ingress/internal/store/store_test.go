package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
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

func TestLoadRefusesACorruptCheckpoint(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at) VALUES ('bad', 'jsonl-replay', 1, X'7B', 'now')`); err != nil {
		t.Fatal(err)
	}

	_, err := s.LoadLine(t.Context(), "bad")

	if err == nil || !strings.Contains(err.Error(), "unmarshal checkpoint") {
		t.Fatalf("err = %v, want a checkpoint decode failure", err)
	}
}

func TestStorageFailureIsNeverAnEmptyCheckpoint(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), `DROP TABLE connector_checkpoints`); err != nil {
		t.Fatal(err)
	}

	_, loadErr := s.LoadLine(t.Context(), "c1")
	saveErr := s.SaveLine(t.Context(), "c1", "jsonl-replay", 1, time.Now())

	if loadErr == nil || !strings.Contains(loadErr.Error(), "load connector checkpoint") {
		t.Fatalf("load err = %v, want a storage failure", loadErr)
	}
	if saveErr == nil || !strings.Contains(saveErr.Error(), "upsert checkpoint") {
		t.Fatalf("save err = %v, want a storage failure", saveErr)
	}
}

func TestSaveLineUpdatesOneRowPerConnectorAndKeepsItsKind(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	first := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Hour)
	for _, save := range []struct {
		kind string
		at   time.Time
	}{{"jsonl-replay", first}, {"simulator-jsonl", later}} {
		if err := s.SaveLine(t.Context(), "c1", save.kind, 5, save.at); err != nil {
			t.Fatal(err)
		}
	}

	var kind, updated string
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT connector_kind, updated_at, (SELECT COUNT(*) FROM connector_checkpoints) FROM connector_checkpoints WHERE connector_id = 'c1'`).Scan(&kind, &updated, &rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || kind != "jsonl-replay" || updated != kernel.FormatTime(later) {
		t.Fatalf("rows = %d, kind = %q, updated_at = %q, want one jsonl-replay row updated at %q", rows, kind, updated, kernel.FormatTime(later))
	}
}

func TestPositionRoundTripsTheLineAndByteOffsetPerConnector(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if position, err := s.LoadPosition(t.Context(), "c1"); err != nil || position != (domain.Checkpoint{}) {
		t.Fatalf("fresh position = %+v err=%v, want the zero checkpoint", position, err)
	}
	want := domain.Checkpoint{LastLine: 12, Offset: 4096}
	if err := s.SavePosition(t.Context(), "c1", "jsonl-replay", want, now); err != nil {
		t.Fatal(err)
	}
	if position, err := s.LoadPosition(t.Context(), "c1"); err != nil || position.LastLine != want.LastLine || position.Offset != want.Offset {
		t.Fatalf("position = %+v err=%v, want %+v", position, err, want)
	}
	if line, err := s.LoadLine(t.Context(), "c1"); err != nil || line != 12 {
		t.Fatalf("line of a position checkpoint = %d err=%v, want 12", line, err)
	}
}

func TestPositionRefusesACorruptCheckpointAndAStorageFailure(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at) VALUES ('bad', 'jsonl-replay', 1, X'7B', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadPosition(t.Context(), "bad"); err == nil || !strings.Contains(err.Error(), "connector checkpoint bad") {
		t.Fatalf("err = %v, want the corrupt checkpoint of bad named", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE connector_checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadPosition(t.Context(), "c1"); err == nil || !strings.Contains(err.Error(), "load connector checkpoint c1") {
		t.Fatalf("load err = %v, want a storage failure", err)
	}
	if err := s.SavePosition(t.Context(), "c1", "jsonl-replay", domain.Checkpoint{LastLine: 1}, time.Now()); err == nil || !strings.Contains(err.Error(), "upsert checkpoint c1") {
		t.Fatalf("save err = %v, want a storage failure", err)
	}
}
