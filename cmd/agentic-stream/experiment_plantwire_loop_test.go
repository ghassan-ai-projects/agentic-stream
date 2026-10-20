package main

import (
	"context"
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
	readingCadence   = 10 * time.Second
	backdatedHistory = 10 * time.Minute
)

var benchLoopEdits = map[string]string{"debounce: 5s": "debounce: 2s", "cooldown: 20s": "cooldown: 2s",
	"policy: approval": "policy: automatic"}

type plantWireLoop struct {
	dir, db, socket string
}

func startPlantWireLoop(t *testing.T, proposals ...proposal) plantWireLoop {
	t.Helper()
	dir := privateSocketDir(t)
	loop := plantWireLoop{dir: dir, db: filepath.Join(dir, "stream.db"), socket: filepath.Join(dir, "host.sock")}
	seedMigratedDatabase(t, loop.db)
	serveWorkerProposing(t, filepath.Join(dir, "worker.sock"), 0, proposals...)
	address := freeLoopbackAddress(t)
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", loop.db, "--spec", editedSpec(t, benchSpec, dir, benchLoopEdits),
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

// zoneWarming sends the zone's readings from backdated history up to now,
// rising 0.3 C a minute from 31 C, and returns the feed that keeps it rising
// in real time.
func zoneWarming(node *plantwirefake.Node) func() {
	start := time.Now().Add(-backdatedHistory)
	send := func(at time.Time) {
		celsius := 31 + 0.3*at.Sub(start).Minutes()
		node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": "zone-01", "boot_id": benchBoot, "historical": false,
			"metrics": []any{benchSample(1, celsius, at.UnixNano()), benchSample(2, 40, at.UnixNano())}})
	}
	for at := start; at.Before(time.Now()); at = at.Add(readingCadence) {
		send(at)
	}
	return func() { send(time.Now()) }
}

// awaitCommand reads the host's records, skipping heartbeats and barriers,
// and keeps the zone warming every second until a DCMD arrives.
func awaitCommand(t *testing.T, node *plantwirefake.Node, warm func()) map[string]any {
	t.Helper()
	deadline := time.Now().Add(loopTimeout)
	for time.Now().Before(deadline) {
		if record, read := node.Next(time.Second); read && record["type"] == "DCMD" {
			return record
		} else if !read {
			warm()
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
	command := awaitCommand(t, node, zoneWarming(node))
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
