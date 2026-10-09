package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const faultLine = `{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"m1","entity":{"type":"motor","id":"m1"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}` + "\n"

func faultFixture(t *testing.T) (*storage.DB, *eventlog.EventLog, string) {
	t.Helper()
	dir := t.TempDir()
	db := storagetest.OpenTemp(t)

	path := filepath.Join(dir, "trace.jsonl")
	if err := os.WriteFile(path, []byte(faultLine), 0o600); err != nil {
		t.Fatal(err)
	}
	return db, eventlog.NewEventLog(db), path
}

// TestCheckpointWriteFailureLeavesTheReplayResumable proves the documented
// at-least-once order: events are appended before the checkpoint, so a failed
// checkpoint write re-reads the trace and the event log deduplicates.
func TestCheckpointWriteFailureLeavesTheReplayResumable(t *testing.T) {
	db, log, path := faultFixture(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fault BEFORE INSERT ON connector_checkpoints BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	conn := newJSONL(t, db, log, "default", path, "fault")
	if _, err := conn.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want the injected storage failure", err)
	}
	var checkpoints int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM connector_checkpoints").Scan(&checkpoints); err != nil || checkpoints != 0 {
		t.Fatalf("checkpoints = %d, %v", checkpoints, err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
		t.Fatal(err)
	}
	if appended, err := conn.Run(t.Context()); err != nil || appended != 0 {
		t.Fatalf("resumed replay appended %d err=%v; the event log must deduplicate", appended, err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM connector_checkpoints").Scan(&checkpoints); err != nil || checkpoints != 1 {
		t.Fatalf("checkpoint rows after recovery = %d, %v", checkpoints, err)
	}
}

func TestStorageFailureReadingACheckpointStopsTheReplay(t *testing.T) {
	db, log, path := faultFixture(t)
	if _, err := db.ExecContext(t.Context(), `DROP TABLE connector_checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := newJSONL(t, db, log, "default", path, "fault").Run(t.Context()); err == nil {
		t.Fatal("a storage failure was treated as an empty checkpoint")
	}
}
