package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// injectFault makes the database refuse one kind of write, simulating a storage
// failure at that write boundary.
func injectFault(t *testing.T, db *storage.DB, trigger string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), trigger); err != nil {
		t.Fatalf("inject fault: %v", err)
	}
}

func count(t *testing.T, db *storage.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEveryDispatchWriteBoundaryRollsBackTheWholeOutcome(t *testing.T) {
	faults := map[string]string{
		"outcome insert":      `CREATE TRIGGER fault BEFORE INSERT ON outcomes BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"command close":       `CREATE TRIGGER fault BEFORE UPDATE ON commands WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"outbox close":        `CREATE TRIGGER fault BEFORE UPDATE ON outbox WHEN NEW.status = 'delivered' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"verification insert": `CREATE TRIGGER fault BEFORE INSERT ON verifications BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"dispatched notice":   `CREATE TRIGGER fault BEFORE INSERT ON notifications WHEN NEW.event_type = '` + notify.CommandDispatched{}.EventType() + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"recorded notice":     `CREATE TRIGGER fault BEFORE INSERT ON notifications WHEN NEW.event_type = '` + notify.OutcomeRecorded{}.EventType() + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"reconciled notice":   `CREATE TRIGGER fault BEFORE INSERT ON notifications WHEN NEW.event_type = '` + notify.OutcomeReconciled{}.EventType() + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
	}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			db, commandID := openActionFixture(t)
			defer func() { _ = db.Close() }()
			injectFault(t, db, trigger)
			service := newService(t, db, &recordingEffector{}, "test-dispatcher", time.Minute)
			if _, err := service.DispatchOnce(t.Context()); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			for _, table := range []string{"outcomes", "verifications", "notifications"} {
				if n := count(t, db, "SELECT COUNT(*) FROM "+table); n != 0 {
					t.Fatalf("%s rows = %d; a failed write must leave nothing behind", table, n)
				}
			}
			var status string
			if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "dispatching" {
				t.Fatalf("command status = %q, %v; it must stay dispatching under its lease", status, err)
			}
		})
	}
}

func TestEveryReconciliationWriteBoundaryRollsBackTheWholeResolution(t *testing.T) {
	faults := map[string]string{
		"outcome insert":      `CREATE TRIGGER fault BEFORE INSERT ON outcomes WHEN NEW.status = 'reconciled' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"command close":       `CREATE TRIGGER fault BEFORE UPDATE ON commands WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"verification update": `CREATE TRIGGER fault BEFORE UPDATE ON verifications BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"reconciled notice":   `CREATE TRIGGER fault BEFORE INSERT ON notifications WHEN NEW.event_type = '` + notify.OutcomeReconciled{}.EventType() + `' BEGIN SELECT RAISE(ABORT, 'injected'); END`,
	}
	evidence := map[string]any{"source": "feedback", "evidence_type": "provider_observation",
		"evidence_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			db, commandID := openActionFixture(t)
			defer func() { _ = db.Close() }()
			service := newService(t, db, &recordingEffector{unknown: true}, "test-dispatcher", time.Minute)
			if _, err := service.DispatchOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
			injectFault(t, db, trigger)
			if err := service.ReconcileUnknown(t.Context(), commandID, "succeeded", evidence); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			if n := count(t, db, "SELECT COUNT(*) FROM outcomes"); n != 1 {
				t.Fatalf("outcomes = %d, want only the original unknown outcome", n)
			}
			var status string
			if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "reconciling" {
				t.Fatalf("command status = %q, %v; it must stay reconciling", status, err)
			}
		})
	}
}
