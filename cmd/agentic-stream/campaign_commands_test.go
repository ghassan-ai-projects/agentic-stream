package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const pumpUnderTest = "site-a:pump-3"

var pumpMetricAliases = map[string]int{"vibration": 1, "axial_vibration": 2, "temperature": 3, "motor_current": 4}

var pumpFastEdits = map[string]string{"maxOutOfOrderness: 2m": "maxOutOfOrderness: 100ms", "size: 15m": "size: 20s", "slide: 1m": "slide: 5s",
	"size: 24h": "size: 1m", "slide: 30m": "slide: 5s", "minDuration: 20m": "minDuration: 100ms", "debounce: 1m": "debounce: 100ms",
	"cooldown: 5m": "cooldown: 100ms"}

func TestTheCampaignsServeFlagsLetAScheduledMaintenanceReachThePlant(t *testing.T) {
	t.Parallel()
	dir := privateSocketDir(t)
	database := filepath.Join(dir, "stream.db")
	seedMigratedDatabase(t, database)
	serveWorker(t, filepath.Join(dir, "worker.sock"), bookBearingWearMaintenance)
	run := plantWireServe{dir: dir, socket: filepath.Join(dir, "host.sock"), db: database}
	servePlantWire(t, run, freeLoopbackAddress(t), editedSpec(t, pumpStationSpec, dir, pumpFastEdits), filepath.Join(pumpStationPlantWire, "kinds"),
		filepath.Join(pumpStationPlantWire, "pump-station.site.json"), campaignSetup{sockets: dir}.serveFlags()...)
	node := bornPumpNode(t, run.socket)
	sendWearingPump(node)
	command := awaitCommand(t, node)
	args, _ := command["args"].(map[string]any)
	if command["device"] != pumpUnderTest || command["command"] != "schedule_maintenance" || args["work_order"] != "bearing_wear" {
		t.Fatalf("DCMD = %v, want schedule_maintenance {work_order: bearing_wear} on %s", command, pumpUnderTest)
	}
	answer(node, command, "DRECEIPT", "status", "accepted")
	answer(node, command, "DRESULT", "reported_outcome", "succeeded")
	waitForRow(t, run.db, "SELECT COUNT(*) FROM outcomes WHERE json_extract(provider_result_json, '$.plant_wire.command') = 'schedule_maintenance'", loopTimeout)
}

func bookBearingWearMaintenance(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	return workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: "bearing_wear", DiagnosisCode: "bearing_wear", Summary: "radial vibration rising",
		Intents: []workerfake.ProposedIntent{{Type: "schedule_maintenance", Risk: "R1", Parameters: map[string]any{"work_order": "bearing_wear"}}}})
}

func bornPumpNode(t *testing.T, socket string) *plantwirefake.Node {
	t.Helper()
	node := plantwirefake.Dial(t, "unix", socket, "pump-station-1")
	node.Send(plantwirefake.Birth(t, filepath.Join(pumpStationPlantWire, "kinds"), "mega-01", "site-a:pump-1", "site-a:pump-2", pumpUnderTest, "site-a:pump-4", "site-a:pump-5", "site-a:pump-6", "site-a:pump-7", "site-a:pump-8"))
	node.Expect("STATE")
	metrics := make([]any, 0, len(pumpMetricAliases))
	for name, alias := range pumpMetricAliases {
		metrics = append(metrics, birthMetric(name, alias, nil, "warming_up"))
	}
	node.Send(deviceBirth(pumpUnderTest, "pump", time.Now().Add(-backdatedHistory).UnixNano(), metrics...))
	return node
}

func sendWearingPump(node *plantwirefake.Node) {
	start := time.Now().Add(-backdatedHistory)
	for at := start; at.Before(time.Now()); at = at.Add(readingCadence) {
		wear := at.After(start.Add(backdatedHistory / 2))
		node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": pumpUnderTest, "boot_id": benchBoot, "historical": false,
			"metrics": []any{benchSample(1, map[bool]float64{false: 2, true: 5}[wear], at.UnixNano()), benchSample(2, 1, at.UnixNano()),
				benchSample(3, 60, at.UnixNano()), benchSample(4, 10, at.UnixNano())}})
	}
}
