package main

import (
	"context"
	"io"
	"testing"
	"time"
)

func pulseSpecAndTrace(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return renamedHeartbeat(t, dir, predictiveSpecPath, "pulse.situation.yaml"), renamedHeartbeat(t, dir, predictiveHeartbeatLog, "pulse.jsonl")
}

func admittedPulseEvents(t *testing.T, database string) int {
	t.Helper()
	var count int
	if err := openReadOnly(t, database).QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_log WHERE event_type = 'motor.pulse.observed'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRunLiveAdmitsEventsOfASchemaFromTheEventSchemasDirectory(t *testing.T) {
	t.Parallel()
	specPath, tracePath := pulseSpecAndTrace(t)
	database := newMigratedDatabasePath(t)
	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"run-live", "--db", database, "--spec", specPath, "--trace", tracePath, "--event-schemas", pulseSchemaDir(t)})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("run-live with --event-schemas: %v", err)
	}
	if admitted := admittedPulseEvents(t, database); admitted == 0 {
		t.Fatal("run-live admitted no event of the directory schema")
	}
}

func TestServeAdmitsEventsOfASchemaFromTheEventSchemasDirectory(t *testing.T) {
	t.Parallel()
	specPath, tracePath := pulseSpecAndTrace(t)
	database := newMigratedDatabasePath(t)
	address := freeLoopbackAddress(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"serve", "--db", database, "--spec", specPath, "--trace", tracePath,
		"--listen", address, "--poll-interval", "10ms", "--event-schemas", pulseSchemaDir(t)})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	waitReady(t, "http://"+address+"/health/live", done)
	if !pollUntil(t, readyTimeout, func() bool { return admittedPulseEvents(t, database) > 0 }) {
		t.Fatal("serve admitted no event of the directory schema")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v after cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}
