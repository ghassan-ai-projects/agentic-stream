package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // read-only view of the runtime database

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

// The real-world-sensor experiment's Agentic Stream slice, run the way
// RUNBOOK-G1 runs it: `serve` with a live normalized socket, a Tamoz worker on
// the worker socket and the emulator effect profile on the device socket. The
// other two processes are stand-ins that speak only the public contracts. See
// docs/unfinished-work-review-2026-10-08/tasks/X01-experiment-compatibility-guard.md.

const (
	experimentSpec    = "../../docs/design/examples/zone-thermal.situation.yaml"
	experimentCatalog = "../../internal/contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json"
	experimentTrace   = "../../examples/thermal-chamber/testdata/trace-opening.jsonl"
)

// experimentRun is one running `serve` with its stand-ins.
type experimentRun struct {
	dir, db, specPath, liveSocket string
	device                        *deviceStandIn
	stop                          func()
}

func TestExperimentClosedLoopThroughServe(t *testing.T) {
	run := startExperiment(t)
	trace := shiftedTrace(t, time.Now().Add(-time.Second))
	feedLive(t, run.liveSocket, trace, "{not json")
	waitForRow(t, run.db, fmt.Sprintf("SELECT (SELECT COUNT(*) FROM event_log) = %d AND (SELECT COUNT(*) FROM event_quarantine) = 1", len(trace)), 60*time.Second)
	waitForRow(t, run.db, "SELECT COUNT(*) FROM verifications", 60*time.Second)

	assertExperimentLedger(t, run.db)
	assertDeviceReceivedPolicyDigest(t, run)
	run.stop()
	assertRunArtifactVerifies(t, run)
}

func startExperiment(t *testing.T) experimentRun {
	t.Helper()
	disableTelemetryExport(t)
	t.Setenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN", "subscriber-secret")
	dir := privateSocketDir(t)
	run := experimentRun{dir: dir, db: filepath.Join(dir, "stream.db"), specPath: tamozActiveSpec(t, dir), liveSocket: filepath.Join(dir, "telemetry.sock")}
	serveTamozStandIn(t, filepath.Join(dir, "worker.sock"))
	run.device = serveDeviceStandIn(t, filepath.Join(dir, "device.sock"), thermalCatalogDigest(t))
	run.stop = startServe(t, run, freeLoopbackAddress(t))
	return run
}

// privateSocketDir is short (sun_path limit) and 0700, as the worker socket
// requires.
func privateSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "as-x01")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// tamozActiveSpec copies the canonical spec with the two edits RUNBOOK-G1
// makes: the Tamoz executor and the active dispatch policy.
func tamozActiveSpec(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(experimentSpec)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "    name: native\n", "    name: tamoz\n    dispatchPolicy: active\n", 1)
	if edited == string(data) {
		t.Fatal("zone-thermal executor block changed; update the runbook edit and this test")
	}
	path := filepath.Join(dir, "zone-thermal-active.situation.yaml")
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil { //nolint:gosec // path is inside the test's own MkdirTemp directory

		t.Fatal(err)
	}
	return path
}

func thermalCatalogDigest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(experimentCatalog)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := device.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func startServe(t *testing.T, run experimentRun, address string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cmd := newServeCommand()
	cmd.SetArgs([]string{
		"--db", run.db, "--spec", run.specPath, "--live-socket", run.liveSocket, "--trace-format", "normalized",
		"--worker-socket", filepath.Join(run.dir, "worker.sock"), "--worker-name", "tamoz",
		"--effect-profile", "emulator", "--device-socket", filepath.Join(run.dir, "device.sock"),
		"--device-catalog", experimentCatalog, "--device-firmware-digest", standInFirmwareDigest,
		"--listen", address, "--poll-interval", "50ms",
	})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	waitReady(t, "http://"+address+"/health/live", done)
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve returned %v after cancellation", err)
		}
	}
	t.Cleanup(stop)
	return stop
}

// shiftedTrace moves the committed thermal trace so its last event happened at
// last, keeping every gap, as the gateway's clock mapping does.
func shiftedTrace(t *testing.T, last time.Time) []string {
	t.Helper()
	data, err := os.ReadFile(experimentTrace)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	offset := last.Sub(eventTime(t, lines[len(lines)-1]))
	shifted := make([]string, len(lines))
	for i, line := range lines {
		shifted[i] = shiftEvent(t, line, offset)
	}
	return shifted
}

func eventTime(t *testing.T, line string) time.Time {
	t.Helper()
	var event struct {
		EventTime time.Time `json:"event_time"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatal(err)
	}
	return event.EventTime
}

func shiftEvent(t *testing.T, line string, offset time.Duration) string {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"event_time", "ingested_at"} {
		at, err := time.Parse(time.RFC3339Nano, event[field].(string))
		if err != nil {
			t.Fatal(err)
		}
		event[field] = at.Add(offset).UTC().Format(time.RFC3339Nano)
	}
	shifted, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(shifted)
}

// feedLive writes lines to the live socket the way the gateway does: one
// normalized envelope per line on a long-lived connection.
func feedLive(t *testing.T, socket string, lines ...any) {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("dial live socket: %v", err)
	}
	defer func() { _ = conn.Close() }()
	writer := bufio.NewWriter(conn)
	for _, line := range flattenLines(lines) {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
}

func flattenLines(items []any) []string {
	var lines []string
	for _, item := range items {
		switch v := item.(type) {
		case string:
			lines = append(lines, v)
		case []string:
			lines = append(lines, v...)
		}
	}
	return lines
}

func openReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func waitForRow(t *testing.T, dbPath, query string, timeout time.Duration) {
	t.Helper()
	db := openReadOnly(t, dbPath)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRowContext(t.Context(), query).Scan(&count); err == nil && count > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no row for %q within %s; ledger: %s", query, timeout, ledgerSummary(t, db))
}

var experimentLedgerTables = []string{"event_log", "event_quarantine", "situations", "trigger_evaluations", "scheduler_items", "episodes", "decisions", "intents", "policy_evaluations", "commands", "outcomes", "verifications"}

func ledgerSummary(t *testing.T, db *sql.DB) string {
	t.Helper()
	parts := make([]string, 0, len(experimentLedgerTables))
	for _, table := range experimentLedgerTables {
		var count int
		_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count) //nolint:gosec // fixed table names
		parts = append(parts, fmt.Sprintf("%s=%d", table, count))
	}
	return strings.Join(parts, " ") + " queue: " + schedulerQueue(t, db) + " episodes: " + episodeStates(t, db)
}

// episodeStates shows each episode's lifecycle and its attempts' terminals.
func episodeStates(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT e.lifecycle_status, COALESCE(e.terminal_json, ''), COALESCE(a.status, ''), COALESCE(a.terminal_json, '')
		FROM episodes e LEFT JOIN episode_attempts a ON a.episode_id = e.episode_id`)
	if err != nil {
		return err.Error()
	}
	defer func() { _ = rows.Close() }()
	var states []string
	for rows.Next() {
		var lifecycle, terminal, attempt, attemptTerminal string
		if rows.Scan(&lifecycle, &terminal, &attempt, &attemptTerminal) == nil {
			states = append(states, fmt.Sprintf("[%s %s attempt=%s %s]", lifecycle, terminal, attempt, attemptTerminal))
		}
	}
	return strings.Join(states, " ")
}

// schedulerQueue shows why queued cognition has or has not started.
func schedulerQueue(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT status, situation_version, COALESCE(not_before, ''), expires_at, created_at FROM scheduler_items ORDER BY created_at")
	if err != nil {
		return err.Error()
	}
	defer func() { _ = rows.Close() }()
	var items []string
	for rows.Next() {
		var status, notBefore, expiresAt, createdAt string
		var version int
		if rows.Scan(&status, &version, &notBefore, &expiresAt, &createdAt) == nil {
			items = append(items, fmt.Sprintf("[%s v%d not_before=%s expires=%s created=%s]", status, version, notBefore, expiresAt, createdAt))
		}
	}
	return strings.Join(items, " ") + " now=" + time.Now().UTC().Format(time.RFC3339Nano)
}
