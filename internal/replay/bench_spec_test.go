package replay_test

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

const benchSpec = "../../examples/real-world-sensor/zone-thermal-bench.situation.yaml"

// The physical bench reports only zone temperature, humidity and a gateway
// heartbeat. Its spec must still open the Situation and reach cooling on a
// sustained rise, which the simulator spec cannot do without ambient readings.
func TestBenchSpecOpensOnTemperatureAndHeartbeatAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	trace := benchShapedTrace(t, dir)
	dbPath := filepath.Join(dir, "bench.db")
	result, err := replay.Run(context.Background(), replay.Request{DBPath: dbPath, SpecPath: benchSpec, TracePath: trace, TenantID: "default"})
	if err != nil {
		t.Fatalf("replay bench trace: %v", err)
	}
	if result.VersionCount == 0 {
		t.Fatal("the bench spec never opened the Situation")
	}
	if phase := finalThermalPhase(t, dbPath); phase != "cooling" {
		t.Fatalf("final phase = %q, want cooling", phase)
	}
	simulator, err := replay.Run(context.Background(), replay.Request{DBPath: filepath.Join(dir, "sim.db"), SpecPath: "../../examples/real-world-sensor/zone-thermal-sim.situation.yaml", TracePath: trace, TenantID: "default"})
	if err != nil || simulator.VersionCount != 0 {
		t.Fatalf("the simulator spec must not open without ambient readings: versions=%d err=%v", simulator.VersionCount, err)
	}
}

// benchShapedTrace keeps only the temperature and heartbeat readings of the
// committed thermal trace, as the bench gateway would report them.
func benchShapedTrace(t *testing.T, dir string) string {
	t.Helper()
	source, err := os.Open(thermalTrace("trace-opening.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	var kept []string
	scanner := bufio.NewScanner(source)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"zone.temp.observed"`) || strings.Contains(line, `"zone.heartbeat.observed"`) {
			kept = append(kept, line)
		}
	}
	path := filepath.Join(dir, "bench-trace.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
