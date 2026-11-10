package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	masterTripped     = "drive_tripped"
	masterLostAmps    = 0.5
	masterShareAmps   = 3.05
	survivorShareAmps = 4.35
	masterLossTripAt  = 30 * time.Second
	masterLossCoast   = 15 * time.Second
	masterLossHistory = 80 * time.Second
)

var (
	masterLossSpec  = filepath.Join(conveyorData, "master-loss.situation.yaml")
	masterLossSite  = filepath.Join(conveyorPlantWire, "belt-conveyor-s6.site.json")
	plantSites      = map[string]string{"s6-stop": masterLossSite, "s6-promote": masterLossSite}
	masterLossSteps = map[string]proposal{"stranded": emptyTheBelt, "followers_lead": limpFeed}
	masterLossCalls = map[string]string{
		"stranded":       "the speed master D-1 tripped and its followers stopped with it: unload the loaded belt by hand, then restart it empty",
		"followers_lead": "the speed master D-1 tripped and a follower took the speed loop: the two survivors carry the belt",
	}
	driveRoles = map[string]string{"D-1": "drive_master", "D-2": "drive_follower", "D-3": "drive_follower"}
)

type masterLoss struct {
	plant    string
	stopAll  bool
	setpoint string
	route    string
	phases   []string
}

func TestTheMasterLossSpecSettlesEveryWindowAndDeclaresNoRestart(t *testing.T) {
	t.Parallel()
	out := runCLI(t, newRootCommand(), "validate", masterLossSpec, "--event-schemas", conveyorEventSchemas)
	if !strings.HasPrefix(out, "ok: master_loss\n") {
		t.Fatalf("validate --event-schemas = %q, want master_loss", out)
	}
	compiled := compileConveyorSpec(t, masterLossSpec)
	for _, window := range compiled.Windows {
		if window.Emit != "on_close" {
			t.Errorf("window %s emits %q, want on_close: an R2 intent is denied on provisional features", window.Name, window.Emit)
		}
	}
	intents := make([]string, len(compiled.Actions.Intents))
	for i, intent := range compiled.Actions.Intents {
		intents[i] = intent.Type
	}
	if want := []string{"install_watch_condition", "reduce_feed", "stop_feed"}; !slices.Equal(intents, want) {
		t.Errorf("intents = %v, want %v: nothing restarts the belt or a drive", intents, want)
	}
	for _, plant := range slices.Sorted(maps.Keys(plantSites)) {
		for route, routed := range readConveyorRoutes(t, plant) {
			if routed.Operation != "set_feed" {
				t.Errorf("plant %s routes %s to %s, want only the feeder's set_feed", plant, route, routed.Operation)
			}
		}
	}
}

var (
	strandedBelt     = masterLoss{plant: "s6-stop", stopAll: true, setpoint: "0", route: "stop_feed", phases: []string{"running", "master_lost", "stranded"}}
	promotedFollower = masterLoss{plant: "s6-promote", setpoint: "54", route: "reduce_feed", phases: []string{"running", "master_lost", "followers_lead"}}
)

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestALostSpeedMasterWhoseFollowersStopWithItStopsTheFeederOfTheStrandedBelt(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	strandedBelt.assertTheLineTakesItsStep(t)
}

func TestALostSpeedMasterWhoseFollowerTakesTheSpeedLoopCutsTheFeedToTheLimpFeed(t *testing.T) {
	t.Parallel()
	promotedFollower.assertTheLineTakesItsStep(t)
}

func (l masterLoss) assertTheLineTakesItsStep(t *testing.T) {
	t.Helper()
	l.assertTheLineTakesItsStepOn(t, startDegradationServe(t, masterLossSpec, l.plant, answerTheMastersTrip(masterLossSteps)))
}

func (l masterLoss) assertTheLineTakesItsStepOn(t *testing.T, run degradationServe) {
	t.Helper()
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	stopApproving := approveWhileRunning(t, run)
	node := masterLossNode(t, run.socket)
	l.play(node, time.Now().Add(-masterLossHistory))
	command := awaitCommand(t, node)
	if args, _ := command["args"].(map[string]any); command["device"] != "F-1" || command["command"] != "set_feed" || args["feed_setpoint"] != json.Number(l.setpoint) {
		t.Fatalf("DCMD = %v, want set_feed {feed_setpoint: %s} on F-1", command, l.setpoint)
	}
	answer(node, command, "DRECEIPT", "status", "accepted")
	answer(node, command, "DRESULT", "reported_outcome", "succeeded")
	node.Send(outputReading("F-1", json.Number(l.setpoint)))
	waitForRow(t, run.db, "SELECT COUNT(*) FROM commands WHERE effector_route = '"+l.route+"' AND status = 'succeeded'", loopTimeout)
	stopApproving()
	assertTwinReadBack(t, run.db, "set_feed", "feed_setpoint", l.setpoint)
	assertPhases(t, run.db, l.phases)
	assertRoutes(t, run.db, []string{l.route})
	assertEveryStepNames(t, run.db, masterTripped)
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
}

func answerTheMastersTrip(steps map[string]proposal) workerfake.ExecuteFunc {
	return func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		var snapshot lineSnapshot
		if err := json.Unmarshal(req.GetSnapshotJson(), &snapshot); err != nil {
			return fmt.Errorf("decode the snapshot: %w", err)
		}
		if snapshot.fact("drive1_amps") >= masterLostAmps {
			return workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: spec.UnknownDiagnosis, DiagnosisCode: spec.UnknownDiagnosis,
				Summary: "the speed master D-1 still carries load"})
		}
		answer := workerfake.Answer{Hypothesis: masterTripped, DiagnosisCode: masterTripped, Summary: masterLossCalls[snapshot.Phase]}
		if step, planned := steps[snapshot.Phase]; planned {
			answer.Intents = []proposal{step}
		}
		return workerfake.Propose(req, emit, answer)
	}
}

func masterLossNode(t *testing.T, socket string) *plantwirefake.Node {
	t.Helper()
	node := plantwirefake.Dial(t, "unix", socket, "belt-1")
	node.Send(plantwirefake.Birth(t, conveyorKinds, "mega-01", conveyorDevices...))
	node.Expect("STATE")
	born := time.Now().Add(-masterLossHistory - time.Minute).UnixNano()
	node.Send(deviceBirth("F-1", "bulk_feeder", born, birthMetric("feed_setpoint", 1, 100, "good"), birthMetric("feed", 2, conveyorFullFeed, "good")))
	node.Send(withBoot(deviceBirth("B-1", "belt_conveyor", born, birthMetric("tail_speed", 1, beltTailRPM, "good"), birthMetric("run_command", 2, 1, "good"))))
	for _, drive := range beltDrives {
		node.Send(withBoot(deviceBirth(drive, driveRoles[drive], born, birthMetric("current", 1, masterShareAmps, "good"),
			birthMetric("winding", 2, beltWindingC, "good"), birthMetric("vibration", 3, beltVibrationMmS, "good"), birthMetric("run_command", 4, 1, "good"))))
	}
	return node
}

func (l masterLoss) play(node *plantwirefake.Node, from time.Time) {
	for at := from; at.Before(time.Now()); at = at.Add(readingCadence) {
		tripped := at.Sub(from) >= masterLossTripAt
		running := !l.stopAll || at.Sub(from) < masterLossTripAt+masterLossCoast
		atNS := at.UnixNano()
		node.Send(beltData("F-1", atNS, 100, conveyorFullFeed*onOff(running)))
		node.Send(beltData("B-1", atNS, beltTailRPM*onOff(running), 1))
		for i, amps := range l.currents(tripped) {
			node.Send(beltData(beltDrives[i], atNS, amps))
		}
	}
}

func (l masterLoss) currents(tripped bool) []float64 {
	switch {
	case !tripped:
		return []float64{masterShareAmps, masterShareAmps, masterShareAmps}
	case l.stopAll:
		return []float64{0, 0, 0}
	}
	return []float64{0, survivorShareAmps, survivorShareAmps}
}

func assertEveryStepNames(t *testing.T, dbPath, diagnosis string) {
	t.Helper()
	query := `SELECT DISTINCT d.validation_status || ' ' || json_extract(d.raw_json, '$.diagnosis_code') FROM decisions d
		JOIN intents i ON i.decision_id = d.decision_id WHERE i.intent_type != 'install_watch_condition'`
	if got := columnValues(t, dbPath, query); !slices.Equal(got, []string{"accepted " + diagnosis}) {
		t.Fatalf("the steps' decisions = %v, want each accepted with the diagnosis %s", got, diagnosis)
	}
}
