package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func meanWindowSpec() spec.CompiledSpec {
	compiled := restartSpec()
	compiled.Windows[0].Size = "10m"
	compiled.Windows[0].Emit = "on_update"
	compiled.Operators[0].Aggregate = "mean"
	return compiled
}

const windowSnapshotSQL = `
	SELECT (SELECT COALESCE(group_concat(event_id || '@' || event_time || '=' || value), '')
	          FROM (SELECT * FROM operator_window_samples ORDER BY event_time, event_id))
	    || '|' || (SELECT COALESCE(group_concat(blob), '') FROM (SELECT CAST(state_blob AS TEXT) AS blob FROM operator_state ORDER BY operator_id, state_key))
	    || '|' || (SELECT COALESCE(group_concat(lower(hex(state_sha256))), '') FROM situations)`

func windowSnapshot(t *testing.T, db *storage.DB) string {
	t.Helper()
	return queryText(t, db, windowSnapshotSQL)
}

func TestWindowStateRestoredFromItsRowsMatchesTheStateKeptInMemory(t *testing.T) {
	t.Parallel()
	clk := sources.NewVirtual(epoch0)
	keptPath, _ := openDatabaseFile(t)
	kept, err := reopenEngine(t, keptPath, meanWindowSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	restartedPath, _ := openDatabaseFile(t)
	for i := range 40 {
		id, at, level := fmt.Sprintf("evt-%02d", i), time.Duration(i)*time.Minute, float64(5+i%7*3)
		appendLevel(t, kept.log, id, at, level)
		runGlobal(t, kept.service)
		restarted, err := reopenEngine(t, restartedPath, meanWindowSpec(), clk)
		if err != nil {
			t.Fatal(err)
		}
		appendLevel(t, restarted.log, id, at, level)
		runGlobal(t, restarted.service)
		closeDatabase(t, restarted.db)
	}
	restarted, err := reopenEngine(t, restartedPath, meanWindowSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	if want, got := windowSnapshot(t, kept.db), windowSnapshot(t, restarted.db); want != got {
		t.Fatalf("state after restarting before every event differs from the state kept in memory:\nkept      %s\nrestarted %s", want, got)
	}
}

func TestWhatAnEventWritesDoesNotGrowWithTheWindow(t *testing.T) {
	t.Parallel()
	rig := newRig(t, meanWindowSpec())
	for i := range 240 {
		appendLevel(t, rig.log, fmt.Sprintf("evt-%03d", i), time.Duration(i)*20*time.Second, float64(i%9))
	}
	runGlobal(t, rig.service)
	if rows := countRows(t, rig.db, "SELECT COUNT(*) FROM operator_window_samples"); rows != 31 {
		t.Fatalf("window sample rows = %d, want the 31 samples of the last 10 minutes at one per 20 seconds", rows)
	}
	if size := countRows(t, rig.db, "SELECT MAX(length(state_blob)) FROM operator_state"); size > 200 {
		t.Fatalf("operator state blob = %d bytes, want it independent of the samples the window holds", size)
	}
}

func TestRolledBackRecordLeavesNoStaleWindowBehind(t *testing.T) {
	t.Parallel()
	rig, clean := newRig(t, meanWindowSpec()), newRig(t, meanWindowSpec())
	for _, r := range []engineRig{rig, clean} {
		appendLevel(t, r.log, "evt-1", 0, 15)
		runGlobal(t, r.service)
		appendLevel(t, r.log, "evt-2", 30*time.Second, 25)
	}
	fault := `CREATE TRIGGER fault BEFORE INSERT ON event_inbox BEGIN SELECT RAISE(ABORT, 'injected'); END`
	if _, err := rig.db.ExecContext(t.Context(), fault); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.service.RunGlobal(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want the injected storage failure", err)
	}
	if _, err := rig.db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
		t.Fatal(err)
	}
	appendLevel(t, rig.log, "evt-3", time.Minute, 65)
	appendLevel(t, clean.log, "evt-3", time.Minute, 65)
	runGlobal(t, rig.service)
	runGlobal(t, clean.service)
	if got, want := windowSnapshot(t, rig.db), windowSnapshot(t, clean.db); got != want {
		t.Fatalf("state after a rolled-back record differs from a run without it:\nrolled back %s\nclean       %s", got, want)
	}
}
