package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestDurableProcessingTimerFiresExactlyOnceAcrossARestart(t *testing.T) {
	t.Parallel()
	path, _ := openDatabaseFile(t)
	clk := sources.NewVirtual(epoch0)
	first, err := reopenEngine(t, path, heartbeatSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	partition := appendHeartbeat(t, first.log, "hb-1", 0, "")
	runGlobal(t, first.service)
	if status := queryText(t, first.db, "SELECT status FROM timers"); status != "pending" {
		t.Fatalf("timer status = %q, want pending until it is due", status)
	}
	closeDatabase(t, first.db)

	second, err := reopenEngine(t, path, heartbeatSpec(), clk)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(5 * time.Minute)
	if fired, err := second.service.RunDueTimers(t.Context(), partition); err != nil || fired != 1 {
		t.Fatalf("fired=%d err=%v, want the restored timer to fire once", fired, err)
	}
	if fired, err := second.service.RunDueTimers(t.Context(), partition); err != nil || fired != 0 {
		t.Fatalf("second firing = %d err=%v, want 0: a fired timer is acknowledged", fired, err)
	}
}

func TestMissingHeartbeatMakesTheSituationUncertainUntilTheHeartbeatReturns(t *testing.T) {
	t.Parallel()
	clk := sources.NewVirtual(epoch0)
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLogWithClock(db, clk)
	compiled := heartbeatSpec()
	service, err := newService(t.Context(), db, log, clk, &compiled, "default", false)
	if err != nil {
		t.Fatal(err)
	}
	partition := appendHeartbeat(t, log, "hb-1", 0, "")
	runGlobal(t, service)
	clk.Advance(5 * time.Minute)
	if fired, err := service.RunDueTimers(t.Context(), partition); err != nil || fired != 1 {
		t.Fatalf("fired=%d err=%v, want 1", fired, err)
	}
	appendHeartbeat(t, log, "hb-2", time.Minute, "")
	runGlobal(t, service)

	completeness := func(version int) string {
		return queryText(t, db, "SELECT completeness FROM situation_versions WHERE version = ?", version)
	}
	if completeness(1) != "uncertain" || completeness(2) != "on_time" {
		t.Fatalf("completeness = %q then %q, want uncertain after the missing heartbeat and on_time after recovery", completeness(1), completeness(2))
	}
	if got := countRows(t, db, "SELECT COUNT(*) FROM timers WHERE status = 'fired'"); got != 1 {
		t.Fatalf("fired timers = %d, want the fired timer to stay fired", got)
	}
	if got := countRows(t, db, "SELECT COUNT(*) FROM timers WHERE status = 'pending'"); got != 1 {
		t.Fatalf("pending timers = %d, want one new timer armed by the returning heartbeat", got)
	}
	state := queryText(t, db, "SELECT CAST(state_json AS TEXT) FROM situations")
	if !strings.Contains(state, `"facts.missing":true`) {
		t.Fatalf("state = %s, want the latest_event_time reducer to retain missing=true", state)
	}
}

func TestTimerOfAPreviousDeviceBootIsRetiredWithoutFiring(t *testing.T) {
	t.Parallel()
	clk := sources.NewVirtual(epoch0)
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLogWithClock(db, clk)
	compiled := heartbeatSpec()
	service, err := newService(t.Context(), db, log, clk, &compiled, "default", false)
	if err != nil {
		t.Fatal(err)
	}
	partition := appendHeartbeat(t, log, "hb-a", 0, "boot-A")
	runGlobal(t, service)
	appendHeartbeat(t, log, "hb-b", time.Minute, "boot-B")
	runGlobal(t, service)
	clk.Advance(6 * time.Minute)

	handled, err := service.RunDueTimers(t.Context(), partition)
	if err != nil || handled != 2 {
		t.Fatalf("handled=%d err=%v, want both the stale and the active timer acknowledged", handled, err)
	}
	if pending := countRows(t, db, "SELECT COUNT(*) FROM timers WHERE status = 'pending'"); pending != 0 {
		t.Fatalf("pending timers = %d, want none", pending)
	}
	if retired := countRows(t, db, "SELECT COUNT(*) FROM operator_state WHERE state_key LIKE 'thing-1' || char(31) || 'boot-A'"); retired != 0 {
		t.Fatalf("retired boot state rows = %d, want none", retired)
	}
	if versions := countRows(t, db, "SELECT COUNT(*) FROM situation_versions"); versions != 1 {
		t.Fatalf("situation versions = %d, want one: only the active boot's timer fires a feature", versions)
	}
}

func TestTimerFailureRollsBackTheFiringAndTheTimerStaysPending(t *testing.T) {
	t.Parallel()
	clk := sources.NewVirtual(epoch0)
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLogWithClock(db, clk)
	compiled := heartbeatSpec()
	service, err := newService(t.Context(), db, log, clk, &compiled, "default", false)
	if err != nil {
		t.Fatal(err)
	}
	partition := appendHeartbeat(t, log, "hb-1", 0, "")
	runGlobal(t, service)
	clk.Advance(5 * time.Minute)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fault BEFORE INSERT ON situation_versions BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunDueTimers(t.Context(), partition); err == nil || !strings.Contains(err.Error(), "run timers transaction") || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want run timers transaction wrapping the injected failure", err)
	}
	if pending := countRows(t, db, "SELECT COUNT(*) FROM timers WHERE status = 'pending'"); pending != 1 {
		t.Fatalf("pending timers = %d, want the timer to stay pending", pending)
	}
	if _, err := db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
		t.Fatal(err)
	}
	if fired, err := service.RunDueTimers(t.Context(), partition); err != nil || fired != 1 {
		t.Fatalf("fired=%d err=%v, want the timer to fire once the fault clears", fired, err)
	}
}

func TestTimerDerivedVersionReachesCognitionInTheFiringTransaction(t *testing.T) {
	t.Parallel()
	clk := sources.NewVirtual(epoch0)
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLogWithClock(db, clk)
	compiled := heartbeatSpec()
	compiled.Cognition = spec.Cognition{Triggers: []spec.Trigger{{
		Name: "silent", When: "features.missing == true", Score: "situation.severity", Threshold: 5, Lane: "fast", MaterialDelta: "delta.phase_changed",
	}}}
	service, err := newService(t.Context(), db, log, clk, &compiled, "default", true)
	if err != nil {
		t.Fatal(err)
	}
	partition := appendHeartbeat(t, log, "hb-1", 0, "")
	runGlobal(t, service)
	if got := countRows(t, db, "SELECT COUNT(*) FROM trigger_evaluations"); got != 0 {
		t.Fatalf("trigger evaluations before the timer fires = %d, want 0", got)
	}
	clk.Advance(5 * time.Minute)
	if fired, err := service.RunDueTimers(t.Context(), partition); err != nil || fired != 1 {
		t.Fatalf("fired=%d err=%v, want 1", fired, err)
	}
	if outcome := queryText(t, db, "SELECT outcome FROM trigger_evaluations"); outcome != "admitted" {
		t.Fatalf("trigger outcome = %q, want admitted for the missing-heartbeat version", outcome)
	}
}
