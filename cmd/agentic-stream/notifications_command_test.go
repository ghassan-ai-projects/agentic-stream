package main

import (
	"strings"
	"testing"
)

func TestNotificationsPruneEnforcesTheFloorAndCountsFirst(t *testing.T) {
	t.Parallel()
	db := newMigratedDatabasePath(t)
	if _, err := runOperatorCommand(t, "notifications", "prune", "--db", db, "--retention", "24h"); err == nil {
		t.Fatal("a one-day retention was accepted")
	}
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"notifications", "prune", "--db", db, "--retention", "720h", "--dry-run"}, "0 would be retired (dry run, nothing changed)"},
		{[]string{"notifications", "prune", "--db", db, "--retention", "720h"}, "notifications: 0 retired"},
		{[]string{"notifications", "prune", "--db", db, "--retention", "720h", "--json"}, `"retired":0`},
	}
	for _, step := range steps {
		out, err := runOperatorCommand(t, step.args...)
		if err != nil || !strings.Contains(out, step.want) {
			t.Fatalf("%v = %q, %v; want %q", step.args, out, err, step.want)
		}
	}
}
