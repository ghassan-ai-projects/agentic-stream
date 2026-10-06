package app_test

import (
	"strings"
	"testing"
)

func TestInstallWriteFailureLeavesNoWatch(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fault BEFORE INSERT ON watch_conditions BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	service := newService(t, db, nil)
	if _, err := service.Dispatch(t.Context(), installable("cmd-1")); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v, want the injected storage failure", err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM watch_conditions").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows = %d, %v", rows, err)
	}
}

func TestFireWriteFailureSpendsNoAllowance(t *testing.T) {
	t.Parallel()
	for name, trigger := range map[string]string{
		"fire record": `CREATE TRIGGER fault BEFORE INSERT ON watch_fires BEGIN SELECT RAISE(ABORT, 'injected'); END`,
		"allowance":   `CREATE TRIGGER fault BEFORE UPDATE ON watch_conditions WHEN NEW.remaining_fires < OLD.remaining_fires BEGIN SELECT RAISE(ABORT, 'injected'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			db := openDB(t)
			service := newService(t, db, nil)
			if _, err := service.Dispatch(t.Context(), installable("cmd-1")); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Fire(t.Context(), "cmd-1", "evt-1", "sit-1", "motor-1", map[string]any{"temperature": 95}); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("err = %v, want the injected storage failure", err)
			}
			var remaining, fires int
			if err := db.QueryRowContext(t.Context(), "SELECT remaining_fires, (SELECT COUNT(*) FROM watch_fires) FROM watch_conditions").Scan(&remaining, &fires); err != nil {
				t.Fatal(err)
			}
			if remaining != 2 || fires != 0 {
				t.Fatalf("remaining=%d fires=%d; a failed fire must record nothing and spend nothing", remaining, fires)
			}
		})
	}
}
