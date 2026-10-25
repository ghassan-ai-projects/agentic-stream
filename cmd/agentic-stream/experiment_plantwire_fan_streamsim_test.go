package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// The fan experiment's simulated rung (PLAN section 7, criteria 1 to 5),
// against the real streamsim node: the experiment's own session plan, paced
// to the wall clock, with its zone_heat_load disturbance moved to the start.

const (
	fanDisturbanceAt     = 5
	fanExperimentTimeout = 5 * time.Minute
)

var thermalModeCooling = proposal{intentType: "select_thermal_mode", risk: "R1", parameters: map[string]any{"mode": "bounded_cooling"}}

func TestTheFanExperimentClosesTheLoopThroughAPacedStreamsimTwin(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	kinds := filepath.Join(repository, "examples", "devices", "thermal-board")
	loop := startPlantWireLoop(t, indicatorAlert, thermalModeCooling)
	startPlannedSession(t, binary, repository, kinds, fanExperimentPlan(t, kinds, loop))
	t.Cleanup(func() { logCommandOutcomes(t, loop.db) })
	for _, route := range []string{"set_indicator", "select_thermal_mode"} {
		waitForRow(t, loop.db, "SELECT COUNT(*) FROM commands WHERE effector_route = '"+route+"' AND status = 'succeeded'", fanExperimentTimeout)
	}
	assertTwinReadBack(t, loop.db, "show_state", "state", "alert")
	assertTwinReadBack(t, loop.db, "set_cooling", "level_percent", "45")
	if quarantined := countRows(t, loop.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
}

// fanExperimentPlan is the committed simulate.plan.json with the host socket of
// this run, the policy digest Agentic Stream logs for the spec it serves, and
// the disturbance at the start of the session.
func fanExperimentPlan(t *testing.T, kinds string, loop plantWireLoop) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(kinds, "fan-experiment", "simulate.plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	disturbance := plan["disturbances"].([]any)[0].(map[string]any)
	disturbance["at_s"] = fanDisturbanceAt
	plan["host_socket"], plan["policies"] = loop.socket, []string{policyDigestOf(t, loop.specPath)}
	return plan
}

func policyDigestOf(t *testing.T, specPath string) string {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), specPath)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// assertTwinReadBack requires an outcome of the plant wire command to be
// verified by the twin's own reading of metric at value.
func assertTwinReadBack(t *testing.T, dbPath, command, metric, value string) {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM outcomes WHERE json_extract(provider_result_json, '$.plant_wire.command') = ?
		AND json_extract(observed_effect_json, '$.source') = 'plantwire.reading'
		AND json_extract(observed_effect_json, '$.feedback.readings[0].metric') = ?
		AND CAST(json_extract(observed_effect_json, '$.feedback.readings[0].observed') AS TEXT) = ?`
	if err := openReadOnly(t, dbPath).QueryRowContext(t.Context(), query, command, metric, value).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("no outcome of %s verified by %s = %s", command, metric, value)
	}
}

// logCommandOutcomes shows each command's status and outcome when the test
// failed, so a run against the twin says where the loop stopped.
func logCommandOutcomes(t *testing.T, dbPath string) {
	t.Helper()
	if !t.Failed() {
		return
	}
	rows, err := openReadOnly(t, dbPath).QueryContext(context.Background(), `SELECT c.effector_route, c.status, o.status,
		COALESCE(CAST(o.provider_result_json AS TEXT), ''), COALESCE(CAST(o.observed_effect_json AS TEXT), '')
		FROM commands c LEFT JOIN outcomes o ON o.command_id = c.command_id ORDER BY o.occurred_at`)
	if err != nil {
		t.Logf("read the commands: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var route, status, outcome, provider, observed string
		if err := rows.Scan(&route, &status, &outcome, &provider, &observed); err == nil {
			t.Logf("command %s %s, outcome %s: provider %s; observed %s", route, status, outcome, provider, observed)
		}
	}
}
