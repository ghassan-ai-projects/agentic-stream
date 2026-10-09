package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func runOperatorCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCommand()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func TestInterlockTripAndClear(t *testing.T) {
	t.Parallel()
	db := newMigratedDatabasePath(t)
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"interlock", "status", "--db", db}, "interlock: ready (version 1"},
		{[]string{"interlock", "trip", "--db", db, "--reason", "fan smells hot"}, "interlock: tripped (version 2"},
		{[]string{"interlock", "status", "--db", db, "--json"}, `"Status":"tripped"`},
		{[]string{"interlock", "clear", "--db", db, "--reason", "inspected"}, "interlock: ready (version 3"},
	}
	for _, step := range steps {
		out, err := runOperatorCommand(t, step.args...)
		if err != nil || !strings.Contains(out, step.want) {
			t.Fatalf("%v = %q, %v; want %q", step.args, out, err, step.want)
		}
	}
	if _, err := runOperatorCommand(t, "interlock", "trip", "--db", db); err == nil {
		t.Fatal("a trip without a reason was accepted")
	}
}

// A running runtime holds the owner lease: the emergency stop must still work,
// but reopening the action plane must wait until the runtime stops.
func TestInterlockClearWaitsForTheRuntimeButTripDoesNot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := storagetest.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	running := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "serve", Lease: time.Minute}
	if err := running.Claim(t.Context(), "epoch-serve"); err != nil {
		t.Fatal(err)
	}
	if out, err := runOperatorCommand(t, "interlock", "trip", "--db", path, "--reason", "stop now"); err != nil {
		t.Fatalf("trip while the runtime runs = %q, %v", out, err)
	}
	if _, err := runOperatorCommand(t, "interlock", "clear", "--db", path, "--reason", "too early"); err == nil || !strings.Contains(err.Error(), "claim runtime ownership") {
		t.Fatalf("clear while the runtime runs = %v, want an ownership refusal", err)
	}
}
