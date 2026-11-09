package main

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	conveyorFullFeed     = 41.7
	conveyorDriveAmps    = 4.2
	conveyorSurvivorAmps = 6.2
	conveyorSetpointLimp = "18"
	conveyorRouteLease   = "5m"
)

var (
	conveyorData         = filepath.Join("..", "..", "examples", "belt-conveyor", "testdata")
	conveyorSpec         = filepath.Join(conveyorData, "line-overload.situation.yaml")
	conveyorEventSchemas = filepath.Join(conveyorData, "event-schemas")
	conveyorPlantWire    = filepath.Join(conveyorData, "plant-wire")
	conveyorKinds        = filepath.Join(conveyorPlantWire, "kinds")
	conveyorSite         = filepath.Join(conveyorPlantWire, "belt-conveyor.site.json")
	conveyorCatalog      = filepath.Join(conveyorPlantWire, "belt-conveyor.catalog.json")
	conveyorDevices      = []string{"F-1", "B-1", "D-1", "D-2", "D-3"}
	limpFeed             = proposal{Type: "reduce_feed", Risk: "R1", Parameters: map[string]any{"level": "limp_b"}}
)

func TestTheLineOverloadSpecCompilesOnlyWithTheConveyorEventSchemas(t *testing.T) {
	t.Parallel()
	cmd := newValidateCommand()
	cmd.SetArgs([]string{conveyorSpec})
	if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), `unknown event schema "conveyor.drive1_current.observed/1.0"`) {
		t.Fatalf("validate without the conveyor schemas = %v, want the drive current schema unknown", err)
	}
	out := runCLI(t, newRootCommand(), "validate", conveyorSpec, "--event-schemas", conveyorEventSchemas)
	if !strings.HasPrefix(out, "ok: line_overload\n") || !strings.Contains(out, "\npolicy_digest: "+conveyorPolicyDigest(t, conveyorSpec)+"\n") {
		t.Fatalf("validate --event-schemas = %q, want line_overload and its policy digest", out)
	}
}

func conveyorPolicyDigest(t *testing.T, specPath string) string {
	t.Helper()
	digest, err := policy.DigestForVersion(compileConveyorSpec(t, specPath).Digest)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func compileConveyorSpec(t *testing.T, specPath string) *spec.CompiledSpec {
	t.Helper()
	schemas, err := spec.LoadEventSchemas(conveyorEventSchemas)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), specPath, spec.WithEventSchemas(schemas))
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestALostDriveAtFullFeedSetsTheFeederToTheLimpFeedAndItsReadBackVerifiesIt(t *testing.T) {
	t.Parallel()
	run := startConveyorServe(t)
	node := conveyorNode(t, run.socket)
	lostDrive(node, time.Now().Add(-time.Minute))
	command := awaitCommand(t, node)
	if args, _ := command["args"].(map[string]any); command["device"] != "F-1" || command["command"] != "set_feed" || args["feed_setpoint"] != json.Number(conveyorSetpointLimp) {
		t.Fatalf("DCMD = %v, want set_feed {feed_setpoint: 18} on F-1", command)
	}
	answer(node, command, "DRECEIPT", "status", "accepted")
	answer(node, command, "DRESULT", "reported_outcome", "succeeded")
	node.Send(outputReading("F-1", 18))
	waitForRow(t, run.db, "SELECT COUNT(*) FROM commands WHERE effector_route = 'reduce_feed' AND status = 'succeeded'", loopTimeout)
	assertTwinReadBack(t, run.db, "set_feed", "feed_setpoint", conveyorSetpointLimp)
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
}

func TestTheLineOverloadSpecAdmitsEveryReadingOfTheSharedSite(t *testing.T) {
	t.Parallel()
	run := startConveyorServe(t)
	line := connectBeltLine(t, degradationServe{plantWireServe: run}, func(int) beltDisturbance { return beltDisturbance{} })
	line.runUntil(t, 3, func(time.Time) bool { return false })
	line.phaseOnceProcessed(t)
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none: the spec must register every event type the site gives", quarantined)
	}
	for _, eventType := range siteEventTypes(t) {
		waitForRow(t, run.db, "SELECT COUNT(*) FROM event_log WHERE event_type = '"+eventType+"'", loopTimeout)
	}
}

func siteEventTypes(t *testing.T) []string {
	t.Helper()
	var site struct {
		Readings []struct {
			EventType string `json:"event_type"`
		} `json:"readings"`
	}
	decodeFile(t, conveyorSite, &site)
	eventTypes := make([]string, len(site.Readings))
	for i, reading := range site.Readings {
		eventTypes[i] = reading.EventType
	}
	return eventTypes
}

func startConveyorServe(t *testing.T) plantWireServe {
	t.Helper()
	workerSocket := filepath.Join(privateSocketDir(t), "worker.sock")
	serveWorker(t, workerSocket, relieveTheLine)
	return startPlantWireServe(t, conveyorSpec, conveyorKinds, conveyorSite, "--device-catalog", conveyorCatalog,
		"--dispatch-lease", conveyorRouteLease, "--worker-socket", workerSocket, "--worker-name", "tamoz", "--event-schemas", conveyorEventSchemas)
}

func relieveTheLine(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	return workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: "drive_tripped", DiagnosisCode: "drive_tripped",
		Summary: "one drive lost its current at full feed", Intents: []proposal{limpFeed}})
}

func conveyorNode(t *testing.T, socket string) *plantwirefake.Node {
	t.Helper()
	node := plantwirefake.Dial(t, "unix", socket, "belt-1")
	node.Send(plantwirefake.Birth(t, conveyorKinds, "mega-01", conveyorDevices...))
	node.Expect("STATE")
	born := time.Now().Add(-backdatedHistory).UnixNano()
	node.Send(deviceBirth("F-1", "bulk_feeder", born, birthMetric("feed_setpoint", 1, 100, "good"), birthMetric("feed", 2, conveyorFullFeed, "good")))
	node.Send(deviceBirth("B-1", "belt_conveyor", born, birthMetric("tail_speed", 1, 76.5, "good")))
	for _, drive := range conveyorDevices[2:] {
		birth := deviceBirth(drive, "conveyor_drive", born, birthMetric("current", 1, 4.2, "good"))
		birth["boot_id"] = bootOf(drive)
		node.Send(birth)
	}
	return node
}

func bootOf(device string) string {
	if device == "F-1" {
		return benchBoot
	}
	return device + "-boot-1"
}

func lostDrive(node *plantwirefake.Node, from time.Time) {
	trip := from.Add(time.Since(from) / 2)
	for at := from; at.Before(time.Now()); at = at.Add(readingCadence) {
		node.Send(conveyorReading("F-1", 2, conveyorFullFeed, at))
		for i, amps := range driveCurrents(!at.Before(trip)) {
			node.Send(conveyorReading(conveyorDevices[2+i], 1, amps, at))
		}
	}
}

func driveCurrents(tripped bool) []float64 {
	if tripped {
		return []float64{conveyorSurvivorAmps, conveyorSurvivorAmps, math.Copysign(0, -1)}
	}
	return []float64{conveyorDriveAmps, conveyorDriveAmps, conveyorDriveAmps}
}

func conveyorReading(device string, alias int, value float64, at time.Time) map[string]any {
	return map[string]any{"type": "DDATA", "lane": "data", "device": device, "boot_id": bootOf(device), "historical": false,
		"metrics": []any{benchSample(alias, value, at.UnixNano())}}
}
