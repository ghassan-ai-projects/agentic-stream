package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const pumpStationSpec = "../../examples/pump-station/pump-station.situation.yaml"

var (
	pumpStationPlantWire    = filepath.Join("..", "..", "examples", "pump-station", "testdata", "plant-wire")
	pumpStationEventSchemas = filepath.Join("..", "..", "examples", "pump-station", "event-schemas")
)

func compilePumpStation(t *testing.T) *spec.CompiledSpec {
	t.Helper()
	schemas, err := spec.LoadEventSchemas(pumpStationEventSchemas)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), pumpStationSpec, spec.WithEventSchemas(schemas))
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
		filepath.Join(pumpStationPlantWire, "pump-station.site.json"), "--event-schemas", pumpStationEventSchemas)
	plan := writePumpStationPlan(t, run.socket, map[string]any{"at_s": 3600, "instance": "site-a/pump-3", "fault": "bearing_wear"})
	station := filepath.Join(repository, "examples", "pump-station")
	node := exec.Cmd{Path: binary, Args: []string{binary, "plantwire", "connect", "--plan", plan, "--domains-dir", station,
		"--kinds-dir", filepath.Join(station, "kinds"), "--adapters-dir", filepath.Join(repository, "adapters"), "--slice-s", "60", "--out", filepath.Join(run.dir, "run")}}
	if err := node.Start(); err != nil {
		t.Fatalf("start the pump station node: %v", err)
	}
	t.Cleanup(func() {
		_ = node.Process.Kill()
		_ = node.Wait()
	})
	opened := func() bool { return openedConditions(t, run.db)["site-a:pump-3"] > 0 }
	if !pollUntil(t, 10*time.Minute, opened) {
		t.Fatalf("pump_condition situations per pump = %v, want one on the worn site-a:pump-3", openedConditions(t, run.db))
	}
	if others := len(openedConditions(t, run.db)); others != 1 {
		t.Fatalf("pump_condition situations per pump = %v, want only the worn site-a:pump-3", openedConditions(t, run.db))
	}
}

func writePumpStationPlan(t *testing.T, socket string, disturbances ...map[string]any) string {
	t.Helper()
	policyDigest, err := policy.DigestForVersion(compilePumpStation(t).Digest)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := json.Marshal(map[string]any{"schema_version": "0.1", "domain": "pump-station", "seed": 7,
		"host_socket": socket, "policies": []string{policyDigest}, "disturbances": disturbances})
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
