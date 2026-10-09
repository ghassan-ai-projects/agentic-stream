package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func latenessSpec(latePolicy string) spec.CompiledSpec {
	compiled := restartSpec()
	compiled.Time = spec.TimePolicy{MaxOutOfOrderness: "0s", AllowedLateness: "30m", LatePolicy: latePolicy}
	compiled.Windows[0].Size = "2h"
	return compiled
}

func TestALateEventChangesStateOnlyWhenItsPolicyCorrectsWithinAllowedLateness(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, policy    string
		lateBy          time.Duration
		wantDisposition string
		wantInWindow    int
	}{
		{"correct within allowed lateness", "correct", 5 * time.Minute, "corrected", 1},
		{"correct and reconsider within allowed lateness", "correct_and_reconsider", 5 * time.Minute, "corrected", 1},
		{"history only", "history_only", 5 * time.Minute, "history_only", 0},
		{"drop with audit", "drop_with_audit", 5 * time.Minute, "dropped", 0},
		{"correct beyond allowed lateness", "correct", 40 * time.Minute, "beyond_allowed_lateness", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rig := newRig(t, latenessSpec(tc.policy))
			appendLevel(t, rig.log, "evt-on-time", time.Hour, 5)
			appendLevel(t, rig.log, "evt-late", time.Hour-tc.lateBy, 50)
			runGlobal(t, rig.service)
			if got := queryText(t, rig.db, "SELECT disposition || '|' || late_policy FROM event_time_dispositions WHERE event_id = 'evt-late'"); got != tc.wantDisposition+"|"+tc.policy {
				t.Fatalf("late event audit = %q, want %s under %s", got, tc.wantDisposition, tc.policy)
			}
			if got := countRows(t, rig.db, "SELECT COUNT(*) FROM operator_state WHERE CAST(state_blob AS TEXT) LIKE '%evt-late%'"); got != tc.wantInWindow {
				t.Fatalf("operator states holding the late event = %d, want %d", got, tc.wantInWindow)
			}
			if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox WHERE event_id = 'evt-late'"); got != 1 {
				t.Fatalf("late event inbox rows = %d, want it marked processed", got)
			}
		})
	}
}

func TestAnOnTimeEventLeavesNoLateAudit(t *testing.T) {
	t.Parallel()
	rig := newRig(t, latenessSpec("drop_with_audit"))
	appendLevel(t, rig.log, "evt-1", time.Hour, 50)
	appendLevel(t, rig.log, "evt-2", time.Hour, 60)
	runGlobal(t, rig.service)
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_time_dispositions"); got != 0 {
		t.Fatalf("late events = %d, want none for events at the watermark", got)
	}
}

func TestAnUnopenedSituationSurvivesARestartAndIsForgottenOnceItPublishes(t *testing.T) {
	t.Parallel()
	compiled := restartSpec()
	_, db := openDatabaseFile(t)
	log := eventlog.NewEventLog(db)
	service, err := newService(t.Context(), db, log, sources.Physical(), &compiled, "default", false)
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, log, "evt-quiet", 0, 5)
	runGlobal(t, service)
	if got := queryText(t, db, `SELECT phase || '|' || (CAST(state_json AS TEXT) LIKE '%"facts.level":5%') FROM unopened_situations`); got != "candidate|1" {
		t.Fatalf("unopened situation = %q, want the candidate state holding the level fact", got)
	}
	restarted, err := newService(t.Context(), db, log, sources.Physical(), &compiled, "default", false)
	if err != nil {
		t.Fatalf("restart over an unopened situation: %v", err)
	}
	appendLevel(t, log, "evt-loud", 30*time.Second, 50)
	runGlobal(t, restarted)
	if unopened, opened := countRows(t, db, "SELECT COUNT(*) FROM unopened_situations"), countRows(t, db, "SELECT COUNT(*) FROM situations"); unopened != 0 || opened != 1 {
		t.Fatalf("unopened = %d, opened = %d after the first publication, want 0 and 1", unopened, opened)
	}
}

func TestASkewedDeviceCannotPushItsNeighborsIntoLateness(t *testing.T) {
	t.Parallel()
	compiled := latenessSpec("correct")
	compiled.Time.ClockSkewTolerance = "5m"
	rig := newRig(t, compiled)
	skewed := thingEnvelope("evt-skewed", "test.level", 24*time.Hour, map[string]any{"level": 5.0})
	skewed.IngestedAt = epoch0
	appendEnvelope(t, rig.log, skewed)
	neighbor := thingEnvelope("evt-neighbor", "test.level", time.Minute, map[string]any{"level": 50.0})
	neighbor.Entity.ID = "thing-2"
	appendEnvelope(t, rig.log, neighbor)
	runGlobal(t, rig.service)
	if got := queryText(t, rig.db, "SELECT group_concat(event_id || ':' || disposition) FROM event_time_dispositions"); got != "evt-skewed:clock_skew" {
		t.Fatalf("dispositions = %q, want only the skewed event refused", got)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM operator_state WHERE CAST(state_blob AS TEXT) LIKE '%evt-neighbor%'"); got != 1 {
		t.Fatalf("operator states holding the neighbor's event = %d, want it applied on time", got)
	}
}
