package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

// The fan experiment's closed loop through `serve`: a plant wire node feeds the
// bench readings, the bench spec opens a situation, a Tamoz-shaped worker
// proposes thermal intents, and the plant wire host commands the node.

const (
	benchBoot        = "boot-A"
	loopTimeout      = 90 * time.Second
	readingCadence   = 5 * time.Second
	backdatedHistory = 2 * time.Minute
)

// benchLoopEdits speed the bench spec up for a test (cognition gates, phase
// dwell times and the out-of-orderness bound) and make the fan intent R1
// automatic: the node approves it (D9).
var benchLoopEdits = map[string]string{"debounce: 5s": "debounce: 100ms", "cooldown: 20s": "cooldown: 100ms",
	"minDuration: 1m": "minDuration: 20s", "minDuration: 30s": "minDuration: 100ms", "maxOutOfOrderness: 15s": "maxOutOfOrderness: 100ms",
	"      risk: R2": "      risk: R1", "policy: approval": "policy: automatic"}

type plantWireLoop struct {
	dir, db, socket, specPath string
}

func startPlantWireLoop(t *testing.T, proposals ...proposal) plantWireLoop {
	t.Helper()
	dir := privateSocketDir(t)
	loop := plantWireLoop{dir: dir, db: filepath.Join(dir, "stream.db"), socket: filepath.Join(dir, "host.sock"),
		specPath: editedSpec(t, benchSpec, dir, benchLoopEdits)}
	seedMigratedDatabase(t, loop.db)
	serveWorkerProposing(t, filepath.Join(dir, "worker.sock"), 0, proposals...)
	address := freeLoopbackAddress(t)
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", loop.db, "--spec", loop.specPath,
		"--worker-socket", filepath.Join(dir, "worker.sock"), "--worker-name", "tamoz", "--effect-profile", "emulator",
		"--plant-wire-socket", loop.socket, "--plant-wire-kinds", filepath.Join(benchPlantWire, "kinds"),
		"--plant-wire-site", filepath.Join(benchPlantWire, "thermal-bench.site.json"),
		"--device-catalog", filepath.Join(benchPlantWire, "thermal-bench.catalog.json"),
		"--listen", address, "--poll-interval", "50ms"})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	waitReady(t, "http://"+address+"/health/live", done)
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve returned %v after cancellation", err)
		}
	})
	return loop
}

// benchNode is the bench board as a fake plant wire node: the zone sensor,
// the indicator showing off and the fan at rest, all on one boot.
func benchNode(t *testing.T, socket string) *plantwirefake.Node {
	t.Helper()
	node := plantwirefake.Dial(t, "unix", socket, "bench-1")
	node.Send(plantwirefake.Birth(t, filepath.Join(benchPlantWire, "kinds"), "mega-01", "zone-01", "led-01", "fan-01"))
	node.Expect("STATE")
	born := time.Now().Add(-backdatedHistory).UnixNano()
	node.Send(deviceBirth("zone-01", "thermal_sensor", born, birthMetric("temperature", 1, nil, "warming_up"), birthMetric("humidity", 2, nil, "warming_up")))
	node.Send(deviceBirth("led-01", "indicator", born, birthMetric("state", 1, "off", "good")))
	node.Send(deviceBirth("fan-01", "fan", born, birthMetric("level_percent", 1, 0, "good")))
	return node
}

func deviceBirth(device, kind string, bornNS int64, metrics ...any) map[string]any {
	return map[string]any{"type": "DBIRTH", "lane": "data", "device": device, "kind": kind, "board": "mega-01",
		"boot_id": benchBoot, "metadata_version": 1, "birth_time_ns": bornNS, "metrics": metrics}
}

func birthMetric(name string, alias int, value any, quality string) map[string]any {
	return map[string]any{"name": name, "alias": alias, "value": value, "quality": quality}
}

func outputReading(device string, value any) map[string]any {
	return map[string]any{"type": "DDATA", "lane": "data", "device": device, "boot_id": benchBoot, "historical": false,
		"metrics": []any{map[string]any{"alias": 1, "value": value, "quality": "good", "event_time_ns": time.Now().UnixNano(), "time_source": "device"}}}
}

// zoneWarming sends the zone's backdated temperatures, rising 8.4 C an hour
// from 31 C up to now: enough to open the situation and watch it.
func zoneWarming(node *plantwirefake.Node) {
	start := time.Now().Add(-backdatedHistory)
	for at := start; at.Before(time.Now()); at = at.Add(readingCadence) {
		node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": "zone-01", "boot_id": benchBoot, "historical": false,
			"metrics": []any{benchSample(1, 31+0.14*at.Sub(start).Minutes(), at.UnixNano())}})
	}
}

// awaitCommand reads the host's records, skipping heartbeats and barriers,
// until a DCMD arrives.
func awaitCommand(t *testing.T, node *plantwirefake.Node) map[string]any {
	t.Helper()
	for deadline := time.Now().Add(loopTimeout); time.Now().Before(deadline); {
		if record, read := node.Next(time.Second); read && record["type"] == "DCMD" {
			return record
		}
	}
	t.Fatal("the host sent no command before the loop timeout")
	return nil
}

func answer(node *plantwirefake.Node, command map[string]any, recordType, member, value string) {
	node.Send(map[string]any{"type": recordType, "lane": "control", "command_id": command["command_id"], member: value, "boot_id": benchBoot})
}

func TestAnUnknownPlantWireOutcomeIsReconciledByTheDevicesOwnReading(t *testing.T) {
	t.Parallel()
	loop := startPlantWireLoop(t, indicatorAlert)
	node := benchNode(t, loop.socket)
	zoneWarming(node)
	command := awaitCommand(t, node)
	if command["device"] != "led-01" || command["command"] != "show_state" {
		t.Fatalf("DCMD = %v, want show_state on led-01", command)
	}
	answer(node, command, "DRECEIPT", "status", "accepted")
	answer(node, command, "DRESULT", "reported_outcome", "unknown")
	node.Send(outputReading("led-01", "alert"))
	id := command["command_id"].(string)
	waitForRow(t, loop.db, "SELECT COUNT(*) FROM commands WHERE command_id = '"+id+"' AND status = 'succeeded'", loopTimeout)
	waitForRow(t, loop.db, "SELECT COUNT(*) FROM verifications WHERE command_id = '"+id+"' AND status = 'reconciled'", loopTimeout)
	waitForRow(t, loop.db, "SELECT COUNT(*) FROM outcomes WHERE command_id = '"+id+"' AND json_extract(provider_result_json, '$.source') = 'plantwire.reading'", loopTimeout)
}

func TestTheBenchLoopSendsTheFanItsBoundedLevelAndItsReadingVerifiesIt(t *testing.T) {
	t.Parallel()
	loop := startPlantWireLoop(t, thermalModeCooling)
	node := benchNode(t, loop.socket)
	zoneWarming(node)
	command := awaitCommand(t, node)
	if args, _ := command["args"].(map[string]any); command["device"] != "fan-01" || command["command"] != "set_cooling" || args["level_percent"] != json.Number("45") {
		t.Fatalf("DCMD = %v, want set_cooling {level_percent: 45} on fan-01", command)
	}
	answer(node, command, "DRECEIPT", "status", "accepted")
	answer(node, command, "DRESULT", "reported_outcome", "succeeded")
	node.Send(outputReading("fan-01", 45))
	waitForRow(t, loop.db, "SELECT COUNT(*) FROM commands WHERE effector_route = 'select_thermal_mode' AND status = 'succeeded'", loopTimeout)
	assertTwinReadBack(t, loop.db, "set_cooling", "level_percent", "45")
}
