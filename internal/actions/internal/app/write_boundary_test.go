package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func noticeOf(eventType string) string {
	return `CREATE TRIGGER fault BEFORE INSERT ON notifications WHEN NEW.event_type = '` + eventType + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`
}

func TestEveryDispatchWriteBoundaryRollsBackTheWholeOutcome(t *testing.T) {
	t.Parallel()
	faults := map[string]string{
		"outcome insert":      `CREATE TRIGGER fault BEFORE INSERT ON outcomes BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"command close":       `CREATE TRIGGER fault BEFORE UPDATE ON commands WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"outbox close":        `CREATE TRIGGER fault BEFORE UPDATE ON outbox WHEN NEW.status = 'delivered' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"verification insert": `CREATE TRIGGER fault BEFORE INSERT ON verifications BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"dispatched notice":   noticeOf(notify.CommandDispatched{}.EventType()),
		"recorded notice":     noticeOf(notify.OutcomeRecorded{}.EventType()),
		"reconciled notice":   noticeOf(notify.OutcomeReconciled{}.EventType()),
	}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			execute(t, db, trigger)
			if _, err := newDispatcher(t, db, succeeds()).DispatchOnce(t.Context()); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			for _, table := range []string{"outcomes", "verifications", "notifications"} {
				if n := count(t, db, "SELECT COUNT(*) FROM "+table); n != 0 {
					t.Fatalf("%s rows = %d; a failed write must leave nothing behind", table, n)
				}
			}
			if got := readLedger(t, db, commandID); got.Command != "dispatching" || got.Outbox != "leased" {
				t.Fatalf("ledger = %+v; the command must stay dispatching under its lease", got)
			}
		})
	}
}

func TestEveryReconciliationWriteBoundaryRollsBackTheWholeResolution(t *testing.T) {
	t.Parallel()
	faults := map[string]string{
		"outcome insert":      `CREATE TRIGGER fault BEFORE INSERT ON outcomes WHEN NEW.status = 'reconciled' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"command close":       `CREATE TRIGGER fault BEFORE UPDATE ON commands WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"verification update": `CREATE TRIGGER fault BEFORE UPDATE ON verifications BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"reconciled notice":   noticeOf(notify.OutcomeReconciled{}.EventType()),
	}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			dispatcher := newDispatcher(t, db, failsWith(unknownOutcome))
			dispatchOnce(t, dispatcher)
			execute(t, db, trigger)
			if err := dispatcher.ReconcileUnknown(t.Context(), commandID, "succeeded", independentEvidence()); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			if got := readLedger(t, db, commandID); got != unknownAfterCrash {
				t.Fatalf("ledger = %+v; the command must stay %+v after a failed reconciliation", got, unknownAfterCrash)
			}
		})
	}
}
