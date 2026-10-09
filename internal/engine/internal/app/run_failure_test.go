package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRunGlobalNamesTheStepThatFailed(t *testing.T) {
	t.Parallel()
	t.Run("storage is gone", func(t *testing.T) {
		t.Parallel()
		rig := newRig(t, restartSpec())
		if err := rig.db.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := rig.service.RunGlobal(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "read applied position") {
			t.Fatalf("err = %v, want read applied position", err)
		}
	})
	t.Run("ownership lost while firing a due timer", func(t *testing.T) {
		t.Parallel()
		var denied atomic.Bool
		lost := errors.New("ownership lost")
		owner := func(context.Context, *sql.Tx, string) error {
			if denied.Load() {
				return lost
			}
			return nil
		}
		clk := sources.NewVirtual(epoch0)
		db := storagetest.OpenTemp(t)
		log := eventlog.NewEventLogWithClock(db, clk)
		compiled := heartbeatSpec()
		service, err := newServiceWithOwner(t.Context(), db, log, clk, &compiled, "default", false, owner)
		if err != nil {
			t.Fatal(err)
		}
		appendHeartbeat(t, log, "hb-1", 0, "")
		runGlobal(t, service)
		appendHeartbeat(t, log, "hb-2", time.Minute, "")
		clk.Advance(10 * time.Minute)
		denied.Store(true)
		processed, err := service.RunGlobal(t.Context(), nil)
		if !errors.Is(err, lost) || processed != 0 || !strings.Contains(err.Error(), "run timers before event 2") {
			t.Fatalf("processed=%d err=%v, want run timers before event 2 wrapping the ownership error", processed, err)
		}
	})
}

func TestARecordThatFailsEveryTimeIsSetAsideWithoutHaltingTheLog(t *testing.T) {
	t.Parallel()
	compiled := heartbeatSpec()
	compiled.Operators[0].Duration = "soon"
	rig := newRig(t, compiled)
	appendHeartbeat(t, rig.log, "hb-1", 0, "")
	appendLevel(t, rig.log, "evt-after", time.Minute, 5)
	if _, err := rig.service.RunGlobal(t.Context(), nil); err != nil {
		t.Fatalf("RunGlobal = %v, want the failing record set aside", err)
	}
	failure := queryText(t, rig.db, "SELECT event_id || '|' || step || '|' || (error_text LIKE '%heartbeat duration%') FROM apply_failures WHERE event_id = 'hb-1'")
	if failure != "hb-1|apply operators|1" {
		t.Fatalf("apply failure = %q, want hb-1 refused by its operators naming the heartbeat duration", failure)
	}
	for _, table := range []string{"operator_state", "timers"} {
		if got := countRows(t, rig.db, "SELECT COUNT(*) FROM "+table); got != 0 {
			t.Errorf("%s holds %d rows after the failed record, want none", table, got)
		}
	}
	if got := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox"); got != 2 {
		t.Fatalf("processed events = %d, want both the set-aside record and the one after it", got)
	}
}
