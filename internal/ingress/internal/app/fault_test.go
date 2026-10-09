package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func faultFixture(t *testing.T) (*storage.DB, *eventlog.EventLog, string) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return db, eventlog.NewEventLog(db), writeTrace(t, joinLines(vibrationLine("evt-1", "2026-01-01T00:00:00Z")))
}

func TestCheckpointWriteFailureLeavesTheReplayResumable(t *testing.T) {
	t.Parallel()
	db, log, path := faultFixture(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fault BEFORE INSERT ON connector_checkpoints BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	conn := newJSONL(t, db, log, "default", path, "fault")

	if _, err := conn.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "save checkpoint") || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want the injected failure wrapped by save checkpoint", err)
	}
	if got := countRows(t, db, "connector_checkpoints"); got != 0 {
		t.Fatalf("checkpoints = %d, want none after the failed write", got)
	}

	if _, err := db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
		t.Fatal(err)
	}
	if appended, err := conn.Run(t.Context()); err != nil || appended != 0 {
		t.Fatalf("resumed replay appended %d, err = %v; the event log must deduplicate the first run's event", appended, err)
	}
	if got := countRows(t, db, "connector_checkpoints"); got != 1 {
		t.Fatalf("checkpoint rows after recovery = %d, want 1", got)
	}
	if got := countRows(t, db, "event_log"); got != 1 {
		t.Fatalf("event log holds %d events, want the one from the first run", got)
	}
}

func TestStorageFailureReadingACheckpointStopsTheReplay(t *testing.T) {
	t.Parallel()
	db, log, path := faultFixture(t)
	if _, err := db.ExecContext(t.Context(), `DROP TABLE connector_checkpoints`); err != nil {
		t.Fatal(err)
	}

	appended, err := newJSONL(t, db, log, "default", path, "fault").Run(t.Context())

	if err == nil || !strings.Contains(err.Error(), "load checkpoint") || appended != 0 {
		t.Fatalf("appended %d, err = %v, want load checkpoint to stop the replay before any append", appended, err)
	}
	if got := countRows(t, db, "event_log"); got != 0 {
		t.Fatalf("event log holds %d events, want none", got)
	}
}
