package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func reopenEngine(t *testing.T, path string, compiled spec.CompiledSpec, clk sources.Clock) (engineRig, error) {
	t.Helper()
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	log := eventlog.NewEventLogWithClock(db, clk)
	service, err := newService(t.Context(), db, log, clk, &compiled, "default", false)
	return engineRig{db: db, log: log, service: service}, err
}

func closeDatabase(t *testing.T, db *storage.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRestartRestoresSituationIdentityAndContinuesItsVersions(t *testing.T) {
	t.Parallel()
	path, _ := openDatabaseFile(t)
	clk := sources.NewVirtual(epoch0)
	first, err := reopenEngine(t, path, escalationSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, first.log, "evt-1", 0, 15)
	runGlobal(t, first.service)
	situationID := queryText(t, first.db, "SELECT situation_id FROM situations")
	closeDatabase(t, first.db)

	second, err := reopenEngine(t, path, escalationSpec(), clk)
	if err != nil {
		t.Fatalf("restore engine: %v", err)
	}
	appendLevel(t, second.log, "evt-2", time.Minute, 60)
	if processed := runGlobal(t, second.service); processed != 1 {
		t.Fatalf("second run processed %d, want 1", processed)
	}
	if restored := queryText(t, second.db, "SELECT situation_id FROM situations"); restored != situationID {
		t.Fatalf("situation identity changed across restart: %s then %s", situationID, restored)
	}
	if version := countRows(t, second.db, "SELECT current_version FROM situations"); version != 2 {
		t.Fatalf("current version = %d, want the restored situation to advance from 1 to 2", version)
	}
	if previous := countRows(t, second.db, "SELECT previous_version FROM situation_versions WHERE version = 2"); previous != 1 {
		t.Fatalf("version 2 supersedes version %d, want 1", previous)
	}
}

func TestRestartKeepsReducerStateThatPublishesNoNewVersion(t *testing.T) {
	t.Parallel()
	path, _ := openDatabaseFile(t)
	clk := sources.NewVirtual(epoch0)
	first, err := reopenEngine(t, path, restartSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, first.log, "evt-1", 0, 15)
	appendLevel(t, first.log, "evt-2", time.Minute, 16)
	runGlobal(t, first.service)
	closeDatabase(t, first.db)

	second, err := reopenEngine(t, path, restartSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	versionBefore := countRows(t, second.db, "SELECT current_version FROM situations")
	appendLevel(t, second.log, "evt-3", 2*time.Minute, 20)
	runGlobal(t, second.service)
	state := queryText(t, second.db, "SELECT CAST(state_json AS TEXT) FROM situations")
	if version := countRows(t, second.db, "SELECT current_version FROM situations"); version != versionBefore || !strings.Contains(state, `"facts.level":20`) {
		t.Fatalf("version=%d (was %d) state=%s, want the reducer state persisted without a new version", version, versionBefore, state)
	}
}

func TestRestartRefusesRuntimeStateItCannotTrust(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, mutation, want string
	}{
		{"legacy codec", "UPDATE situations SET state_codec_version = 0", "requires rebuild"},
		{"unknown codec", "UPDATE situations SET state_codec_version = 9", "unsupported state codec 9"},
		{"tampered state", `UPDATE situations SET state_json = CAST(replace(CAST(state_json AS TEXT), '"facts.level":15', '"facts.level":99') AS BLOB)`, "digest mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, _ := openDatabaseFile(t)
			clk := sources.NewVirtual(epoch0)
			first, err := reopenEngine(t, path, restartSpec(), clk)
			if err != nil {
				t.Fatal(err)
			}
			appendLevel(t, first.log, "evt-1", 0, 15)
			runGlobal(t, first.service)
			if _, err := first.db.ExecContext(t.Context(), tc.mutation); err != nil {
				t.Fatal(err)
			}
			closeDatabase(t, first.db)

			if _, err := reopenEngine(t, path, restartSpec(), clk); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want the stored state refused with %q", err, tc.want)
			}
		})
	}
}
