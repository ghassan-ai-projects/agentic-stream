package app_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRunGlobalAppliesAnEventOnceAndAdvancesTheCheckpoint(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	partition := appendLevel(t, rig.log, "evt-1", 0, 15)

	if processed := runGlobal(t, rig.service); processed != 1 {
		t.Fatalf("first run processed %d events, want 1", processed)
	}
	if processed := runGlobal(t, rig.service); processed != 0 {
		t.Fatalf("second run processed %d events, want 0: the inbox and checkpoint already hold it", processed)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox WHERE event_id = 'evt-1'"); got != 1 {
		t.Fatalf("inbox rows = %d, want 1", got)
	}
	if got := countRows(t, rig.db, "SELECT last_position FROM partition_checkpoints WHERE partition_id = ?", partition); got != 1 {
		t.Fatalf("checkpoint position = %d, want the event's log position 1", got)
	}
}

func TestGlobalRunFailurePreservesProgressAndInboxDeduplication(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	appendLevel(t, rig.log, "evt-first", 0, 15)
	appendLevel(t, rig.log, "evt-second", time.Second, 16)
	failure := errors.New("stop before second event")
	processed, err := rig.service.RunGlobal(t.Context(), func(record eventlog.Record) error {
		if record.EventID == "evt-second" {
			return failure
		}
		return nil
	})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "before apply hook") || processed != 1 {
		t.Fatalf("processed=%d err=%v, want one event applied then the hook failure", processed, err)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox"); got != 1 {
		t.Fatalf("inbox rows = %d, want only the first event", got)
	}
	if processed := runGlobal(t, rig.service); processed != 1 {
		t.Fatalf("resumed run processed %d events, want only the one that failed", processed)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox"); got != 2 {
		t.Fatalf("resumed inbox rows = %d, want 2", got)
	}
}

func TestGlobalRunResumesAfterTheAppliedPosition(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	for i := range 10 {
		appendLevel(t, rig.log, fmt.Sprintf("evt-%02d", i), time.Duration(i)*time.Second, 15)
	}
	if processed := runGlobal(t, rig.service); processed != 10 {
		t.Fatalf("first run processed %d, want 10", processed)
	}
	visited := 0
	count := func(eventlog.Record) error { visited++; return nil }
	processed, err := rig.service.RunGlobal(t.Context(), count)
	if err != nil || processed != 0 || visited != 0 {
		t.Fatalf("idle run processed=%d visited=%d err=%v; it must not revisit applied records", processed, visited, err)
	}
	appendLevel(t, rig.log, "evt-new", time.Minute, 16)
	processed, err = rig.service.RunGlobal(t.Context(), count)
	if err != nil || processed != 1 || visited != 1 {
		t.Fatalf("next run processed=%d visited=%d err=%v; it must apply only the new record", processed, visited, err)
	}
}

func TestLateEventIsAppliedButNeverPullsThePartitionWatermarkBack(t *testing.T) {
	t.Parallel()
	compiled := restartSpec()
	compiled.Time.MaxOutOfOrderness = "1m"
	rig := newRig(t, compiled)
	appendLevel(t, rig.log, "evt-on-time", 10*time.Minute, 15)
	runGlobal(t, rig.service)
	watermark := func() string {
		return queryText(t, rig.db, "SELECT watermark FROM partition_checkpoints")
	}
	const onTimeWatermark = "2026-01-01T00:09:00.000000000Z"
	if got := watermark(); got != onTimeWatermark {
		t.Fatalf("watermark after the on-time event = %s, want event time minus the 1m lag: %s", got, onTimeWatermark)
	}

	appendLevel(t, rig.log, "evt-late", 0, 99)
	if processed := runGlobal(t, rig.service); processed != 1 {
		t.Fatalf("late event processed %d, want 1: late data is applied, not dropped", processed)
	}
	if got := watermark(); got != onTimeWatermark {
		t.Fatalf("watermark after the late event = %s, want it held at %s", got, onTimeWatermark)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox WHERE event_id = 'evt-late'"); got != 1 {
		t.Fatalf("late event inbox rows = %d, want 1", got)
	}
	state := queryText(t, rig.db, "SELECT CAST(state_json AS TEXT) FROM situations")
	if !strings.Contains(state, `"facts.level":15`) {
		t.Fatalf("situation state = %s, want the late event not to replace the newer level fact", state)
	}
}

func TestAnEventAlreadyInTheInboxChangesNoState(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	appendLevel(t, rig.log, "evt-1", 0, 15)
	if _, err := rig.db.ExecContext(t.Context(), `INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at)
		VALUES ('engine', 'default', 'evt-1', 1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	runGlobal(t, rig.service)
	for _, table := range []string{"situations", "situation_versions", "operator_state", "partition_checkpoints"} {
		if got := countRows(t, rig.db, "SELECT COUNT(*) FROM "+table); got != 0 {
			t.Errorf("%s holds %d rows, want none: an event the inbox already holds must not be applied again", table, got)
		}
	}
}

func TestConcurrentRunsApplyEachEventExactlyOnce(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	const events = 20
	for i := range events {
		appendLevel(t, rig.log, fmt.Sprintf("evt-%02d", i), time.Duration(i)*time.Second, 15)
	}
	var wg sync.WaitGroup
	processed := make([]int, 4)
	errs := make([]error, len(processed))
	for i := range processed {
		wg.Go(func() { processed[i], errs[i] = rig.service.RunGlobal(t.Context(), nil) })
	}
	wg.Wait()
	total := 0
	for i, n := range processed {
		if errs[i] != nil {
			t.Fatalf("run %d: %v", i, errs[i])
		}
		total += n
	}
	if total != events {
		t.Fatalf("concurrent runs processed %d events in total, want %d: runs must be serialized", total, events)
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox"); got != events {
		t.Fatalf("inbox rows = %d, want %d", got, events)
	}
}

func TestTheSameEvidenceYieldsIdenticalSituationsOnEveryRun(t *testing.T) {
	t.Parallel()
	snapshot := func() string {
		db := storagetest.OpenTemp(t)
		clk := sources.NewVirtual(epoch0)
		log := eventlog.NewEventLogWithClock(db, clk)
		compiled := restartSpec()
		service, err := newService(t.Context(), db, log, clk, &compiled, "default", false)
		if err != nil {
			t.Fatal(err)
		}
		for i, level := range []float64{15, 20, 25} {
			appendLevel(t, log, fmt.Sprintf("evt-%d", i), time.Duration(i)*time.Minute, level)
		}
		runGlobal(t, service)
		return queryText(t, db, `SELECT group_concat(v.situation_id || '|' || v.version || '|' || v.phase || '|' || hex(v.snapshot_sha256) || '|' || v.lineage_id || '|' || v.created_at, ';')
			FROM (SELECT * FROM situation_versions ORDER BY situation_id, version) v`)
	}
	first, second := snapshot(), snapshot()
	if first == "" || first != second {
		t.Fatalf("two runs over the same evidence differ:\n first: %s\nsecond: %s", first, second)
	}
}
