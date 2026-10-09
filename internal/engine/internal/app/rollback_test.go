package app_test

import (
	"strings"
	"testing"
	"time"
)

func TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			rig := newRig(t, restartSpec())
			appendLevel(t, rig.log, "evt-fault", 0, 15)
			if _, err := rig.db.ExecContext(t.Context(), trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := rig.service.RunGlobal(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			for _, table := range []string{"event_inbox", "partition_checkpoints", "situations", "situation_versions", "operator_state"} {
				if n := countRows(t, rig.db, "SELECT COUNT(*) FROM "+table); n != 0 {
					t.Fatalf("%s rows = %d; the record must roll back whole", table, n)
				}
			}
			if _, err := rig.db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
				t.Fatal(err)
			}
			if processed := runGlobal(t, rig.service); processed != 1 {
				t.Fatalf("recovery processed %d, want the event applied once after the fault clears", processed)
			}
			if applied := countRows(t, rig.db, "SELECT COUNT(*) FROM event_inbox"); applied != 1 {
				t.Fatalf("inbox rows = %d, want 1", applied)
			}
		})
	}
}

func TestRolledBackRecordLeavesNoStaleInMemorySituation(t *testing.T) {
	t.Parallel()
	rig := newRig(t, escalationSpec())
	appendLevel(t, rig.log, "evt-1", 0, 15)
	runGlobal(t, rig.service)
	if version := countRows(t, rig.db, "SELECT current_version FROM situations"); version != 1 {
		t.Fatalf("current version after the first event = %d, want 1", version)
	}
	appendLevel(t, rig.log, "evt-2", time.Minute, 60)
	fault := `CREATE TRIGGER fault BEFORE INSERT ON event_inbox BEGIN SELECT RAISE(ABORT, 'injected'); END`
	if _, err := rig.db.ExecContext(t.Context(), fault); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.service.RunGlobal(t.Context(), nil); err == nil {
		t.Fatal("the injected failure was not reported")
	}
	if _, err := rig.db.ExecContext(t.Context(), "DROP TRIGGER fault"); err != nil {
		t.Fatal(err)
	}
	runGlobal(t, rig.service)
	if version := countRows(t, rig.db, "SELECT current_version FROM situations"); version != 2 {
		t.Fatalf("current version = %d, want 2: the rolled-back escalation must not leave the in-memory Situation a version ahead", version)
	}
	if versions := countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions"); versions != 2 {
		t.Fatalf("published versions = %d, want 2", versions)
	}
}
