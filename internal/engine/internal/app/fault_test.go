package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery injects a
// failure at each write boundary of one record's transaction and proves the
// record leaves no trace, the in-memory Situations are rebuilt, and the same
// event then applies exactly once.
func TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery(t *testing.T) {
	faults := map[string]string{
		"inbox insert":      `CREATE TRIGGER fault BEFORE INSERT ON event_inbox BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"checkpoint upsert": `CREATE TRIGGER fault BEFORE INSERT ON partition_checkpoints BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"situation upsert":  `CREATE TRIGGER fault BEFORE INSERT ON situations BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"version insert":    `CREATE TRIGGER fault BEFORE INSERT ON situation_versions BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"lineage insert":    `CREATE TRIGGER fault BEFORE INSERT ON lineage_sets BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"operator state":    `CREATE TRIGGER fault BEFORE INSERT ON operator_state BEGIN SELECT RAISE(ABORT, 'injected'); END`,
	}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "engine.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			compiled := restartSpec()
			log := eventlog.NewEventLog(db)
			eng, err := newService(ctx, db, log, sources.Physical(), &compiled, "default", false)
			if err != nil {
				t.Fatal(err)
			}
			appendLevel(t, ctx, log, "evt-fault", 0, 15)
			if _, err := db.ExecContext(ctx, trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := eng.RunGlobal(ctx, nil); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			for _, table := range []string{"event_inbox", "partition_checkpoints", "situations", "situation_versions", "operator_state"} {
				var n int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil || n != 0 {
					t.Fatalf("%s rows = %d, %v; the record must roll back whole", table, n, err)
				}
			}
			if _, err := db.ExecContext(ctx, "DROP TRIGGER fault"); err != nil {
				t.Fatal(err)
			}
			if processed, err := eng.RunGlobal(ctx, nil); err != nil || processed != 1 {
				t.Fatalf("recovery processed=%d err=%v; the event must apply once after the fault clears", processed, err)
			}
			var applied int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_inbox").Scan(&applied); err != nil || applied != 1 {
				t.Fatalf("inbox rows = %d, %v", applied, err)
			}
		})
	}
}
