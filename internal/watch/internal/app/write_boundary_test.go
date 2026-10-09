package app_test

import (
	"strings"
	"testing"
)

func TestAnInstallWriteFailureLeavesNoWatch(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fault BEFORE INSERT ON watch_conditions BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := newService(t, db).Dispatch(t.Context(), installable("cmd-1")); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want the injected storage failure", err)
	}
	if n := countWatches(t, db); n != 0 {
		t.Fatalf("watches = %d, want none", n)
	}
}

func TestAFireWriteFailureRecordsNothingAndSpendsNoAllowance(t *testing.T) {
	t.Parallel()
	faults := map[string]string{
		"fire record": `CREATE TRIGGER fault BEFORE INSERT ON watch_fires BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"allowance":   `CREATE TRIGGER fault BEFORE UPDATE ON watch_conditions WHEN NEW.remaining_fires < OLD.remaining_fires BEGIN SELECT RAISE(ABORT, 'injected'); END`,
	}
	for name, trigger := range faults {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := openDB(t)
			service := newService(t, db)
			install(t, service, installable("cmd-1"))
			if _, err := db.ExecContext(t.Context(), trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Fire(t.Context(), "cmd-1", "evt-1", "sit-1", "motor-1", map[string]any{"temperature": 95}); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "active", Remaining: 2}); got != want {
				t.Fatalf("watch = %+v, want %+v: a failed fire must record nothing and spend nothing", got, want)
			}
		})
	}
}
