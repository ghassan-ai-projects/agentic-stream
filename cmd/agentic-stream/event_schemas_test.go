package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const (
	predictiveSpecPath     = "../../examples/predictive-maintenance/predictive-maintenance.situation.yaml"
	predictiveHeartbeatLog = "../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl"
	motorPulseSchema       = `{"motor.pulse.observed/1.0": {"event_type": "motor.pulse.observed", "schema_version": "1.0", "fields": {}}}`
)

func renamedHeartbeat(t *testing.T, dir, source, name string) string {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeInto(t, dir, name, strings.ReplaceAll(string(content), "motor.heartbeat.observed", "motor.pulse.observed")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

func pulseSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := writeInto(t, dir, "pulse.json", motorPulseSchema); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunReplaysAgainstTheEventSchemasOfTheRootFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specPath := renamedHeartbeat(t, dir, predictiveSpecPath, "pulse.situation.yaml")
	tracePath := renamedHeartbeat(t, dir, predictiveHeartbeatLog, "pulse.jsonl")
	without := newRootCommand()
	without.SetArgs([]string{"run", "--spec", specPath, "--trace", tracePath, "--db", filepath.Join(dir, "without.db")})
	if err := without.ExecuteContext(seededReplayContext(t)); err == nil || !strings.Contains(err.Error(), `unknown event schema "motor.pulse.observed/1.0"`) {
		t.Fatalf("run without --event-schemas = %v, want the pulse schema unknown", err)
	}
	original := runSeededReplay(t, "--spec", predictiveSpecPath, "--trace", predictiveHeartbeatLog, "--db", filepath.Join(dir, "original.db"))
	pulse := runSeededReplay(t, "--spec", specPath, "--trace", tracePath, "--db", filepath.Join(dir, "pulse.db"), "--event-schemas", pulseSchemaDir(t))
	if eventsProcessed(original) == "" || eventsProcessed(pulse) != eventsProcessed(original) {
		t.Fatalf("run with --event-schemas = %q, want the events of the built-in run %q", pulse, original)
	}
}

func runSeededReplay(t *testing.T, args ...string) string {
	t.Helper()
	var out strings.Builder
	cmd := newRootCommand()
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"run"}, args...))
	if err := cmd.ExecuteContext(seededReplayContext(t)); err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String()
}

func eventsProcessed(output string) string {
	field, _, _ := strings.Cut(output, " ")
	return strings.TrimPrefix(field, "events_processed=")
}

func TestLiveCommandsRefuseAnEventSchemaDirectoryBeforeListening(t *testing.T) {
	t.Parallel()
	broken := t.TempDir()
	if err := writeInto(t, broken, "broken.json", "{"); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"missing": filepath.Join(t.TempDir(), "absent"), "malformed": broken} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			address := freeLoopbackAddress(t)
			database := filepath.Join(t.TempDir(), "stream.db")
			for _, args := range [][]string{
				{"serve", "--db", database, "--spec", experimentSpec, "--trace", experimentTrace, "--listen", address, "--event-schemas", dir},
				{"run-live", "--db", database, "--spec", experimentSpec, "--trace", experimentTrace, "--event-schemas", dir},
			} {
				cmd := newRootCommand()
				cmd.SetArgs(args)
				if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "--event-schemas: load event schemas from "+dir) {
					t.Fatalf("%s with a %s schema directory = %v, want it refused", args[0], name, err)
				}
			}
			conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", address)
			if !errors.Is(err, syscall.ECONNREFUSED) {
				if conn != nil {
					_ = conn.Close()
				}
				t.Fatalf("dial after a refused serve = %v, want connection refused", err)
			}
			if _, err := os.Stat(database); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("a refused start touched the database: %v", err)
			}
		})
	}
}
