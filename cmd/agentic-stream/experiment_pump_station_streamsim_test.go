package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const pumpStationSpec = "../../examples/pump-station/pump-station.situation.yaml"

var (
	pumpStationPlantWire = filepath.Join("..", "..", "examples", "pump-station", "testdata", "plant-wire")
	pumpStationCatalog   = filepath.Join(pumpStationPlantWire, "pump-station.catalog.json")
)

func compilePumpStation(t *testing.T) *spec.CompiledSpec {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), pumpStationSpec)
	if err != nil {
		t.Fatalf("compile the pump-station spec: %v", err)
	}
	return compiled
}

func TestThePumpStationSpecCompilesWithItsPromotionBlock(t *testing.T) {
	t.Parallel()
	compiled := compilePumpStation(t)
	codes, err := compiled.DiagnosisCodes()
	if err != nil || len(codes) != 8 || compiled.Promotion == nil || compiled.Promotion.SettleWindows["overload"] != "2h" {
		t.Fatalf("catalog %v (%v), promotion %+v; want 8 codes and a 2h overload settle window", codes, err, compiled.Promotion)
	}
}

func TestThePumpStationOpensAConditionOnTheWornPump(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	run := startPlantWireServe(t, pumpStationSpec, filepath.Join(pumpStationPlantWire, "kinds"),
		filepath.Join(pumpStationPlantWire, "pump-station.site.json"), "--device-catalog", pumpStationCatalog)
	plan := writePumpStationPlan(t, run.socket, "pump-station", map[string]any{"at_s": 3600, "instance": "site-a/pump-3", "fault": "bearing_wear"})
	startPumpStationNode(t, binary, repository, plan, filepath.Join(run.dir, "run"))
	opened := func() bool { return openedConditions(t, run.db)["site-a:pump-3"] > 0 }
	if !pollUntil(t, 10*time.Minute, opened) {
		t.Fatalf("pump_condition situations per pump = %v, want one on the worn site-a:pump-3", openedConditions(t, run.db))
	}
	if others := len(openedConditions(t, run.db)); others != 1 {
		t.Fatalf("pump_condition situations per pump = %v, want only the worn site-a:pump-3", openedConditions(t, run.db))
	}
}

func startPumpStationNode(t *testing.T, binary, repository, plan, out string) {
	t.Helper()
	station := filepath.Join(repository, "examples", "pump-station")
	node := exec.Cmd{Path: binary, Args: []string{binary, "plantwire", "connect", "--plan", plan, "--domains-dir", station,
		"--kinds-dir", filepath.Join(station, "kinds"), "--adapters-dir", filepath.Join(repository, "adapters"), "--slice-s", "60", "--out", out}}
	if err := node.Start(); err != nil {
		t.Fatalf("start the pump station node: %v", err)
	}
	t.Cleanup(func() {
		_ = node.Process.Kill()
		_ = node.Wait()
	})
}

func writePumpStationPlan(t *testing.T, socket, domain string, disturbances ...map[string]any) string {
	t.Helper()
	plan, err := json.Marshal(map[string]any{"schema_version": "0.1", "domain": domain, "seed": 7,
		"host_socket": socket, "policies": []string{pumpStationPolicyDigest(t)}, "disturbances": disturbances})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session-plan.json")
	if err := os.WriteFile(path, plan, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func openedConditions(t *testing.T, dbPath string) map[string]int {
	t.Helper()
	return countsBy(t, dbPath, "SELECT entity_id, COUNT(*) FROM situations WHERE situation_type = 'pump_condition' GROUP BY entity_id")
}

func TestRulesPinnedToTheOldPumpKindStopAtTheFirstProbeBBirth(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	run := startPlantWireServe(t, pumpStationSpec, filepath.Join(pumpStationPlantWire, "kinds-probe-b"),
		filepath.Join(pumpStationPlantWire, "pump-station.site.json"), "--playbook", filepath.Join(pumpStationPlantWire, "..", "site-a.playbook.yaml"),
		"--device-catalog", pumpStationCatalog)
	plan := writePumpStationPlan(t, run.socket, "pump-station-probe-b", map[string]any{"at_s": 3600, "instance": "site-a/pump-3", "fault": "bearing_wear"})
	startPumpStationNode(t, binary, repository, plan, filepath.Join(run.dir, "run"))
	switchedOff := func() bool {
		return countsBy(t, run.db, "SELECT cause, COUNT(*) FROM rule_circuits WHERE state = 'open' GROUP BY cause")["kind_changed"] == 2
	}
	if !pollUntil(t, 5*time.Minute, switchedOff) {
		t.Fatal("the rules pinned to the old pump kind were not switched off at the probe-b birth")
	}
	if fires := countRows(t, run.db, "rule_fires"); fires != 0 {
		t.Fatalf("%d rule fires on the new kind, want none before a human approves the rules again", fires)
	}
}
