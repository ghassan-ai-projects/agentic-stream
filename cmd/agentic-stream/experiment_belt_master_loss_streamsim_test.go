package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

const masterFault = "drive_master__vfd_fault"

type masterLossTwin struct {
	route    string
	readBack string
	phases   []string
	judge    func(t *testing.T, grade outcomeGrade, output []byte)
}

var masterLossTwins = map[string]masterLossTwin{
	"s6-a-stop": {route: "stop_feed", readBack: "0", phases: []string{"running", "master_lost", "stranded"},
		judge: assertNoRestartWhileLoaded},
	"s6-a-promote": {route: "reduce_feed", readBack: plantLimp, phases: []string{"running", "master_lost", "followers_lead"},
		judge: assertThePromotedFollowersAreSaved},
}

func (m masterLossTwin) planned() twinStory {
	return twinStory{spec: masterLossSpec, routes: []string{m.route}, readBacks: [][3]string{{"set_feed", "feed_setpoint", m.readBack}},
		device: "F-1", diagnosis: masterTripped, fault: masterFault, faulted: "D-1"}
}

type twinOutcome struct {
	db, runDir string
	ran        twinRun
	grade      outcomeGrade
	output     []byte
}

func (m masterLossTwin) assertTheOutcome(t *testing.T, played twinOutcome) {
	t.Helper()
	assertPhases(t, played.db, m.phases)
	assertEveryStepNames(t, played.db, masterTripped)
	m.judge(t, played.grade, played.output)
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestTheMasterLossLoopPlaysBothS6StoriesOnABarrierPacedStreamsimTwin(t *testing.T) {
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	table, rows := twin.storyTable(t), twinRows(t)
	for _, name := range slices.Sorted(maps.Keys(masterLossTwins)) {
		story := table.story(t, name)
		limp := twin.limpSetpoint(t, table, story)
		t.Run(name, func(t *testing.T) {
			for i, row := range story.firstRows(rows) {
				play := twinPlay{name: name, story: story, row: row, limp: limp}
				t.Run(fmt.Sprintf("row-%d", i+1), func(t *testing.T) { twin.playMasterLoss(t, play, masterLossTwins[name]) })
			}
		})
	}
}

func (c conveyorTwin) playMasterLoss(t *testing.T, play twinPlay, story masterLossTwin) {
	t.Helper()
	started := time.Now()
	t.Cleanup(func() {
		t.Logf("%s seed %d played in %s of wall time", play.name, play.seed(), time.Since(started).Round(time.Second))
	})
	planned := story.planned()
	run := startDegradationServe(t, planned.specPath(), play.story.Plant, answerTheMastersTrip(masterLossSteps))
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	plan := c.sessionPlan(t, play.story.ID, run.socket, conveyorPolicyDigest(t, planned.specPath()), play.story.planOf(play.row))
	stopApproving := approveWhileRunning(t, run)
	runDir := c.connect(t, play.name, plan, "--slice-s", barrierSliceS, "--barrier-cap-ms", barrierCapMS)
	stopApproving()
	ran := readTwinRun(t, runDir)
	ran.assertSeed(t, play.seed())
	trip := ran.onsetOf(t, planned.faulted)
	logPhasesSince(t, run.db, trip)
	logCommandsSince(t, run.db, trip)
	assertRoutes(t, run.db, planned.routes)
	planned.assertReadBacks(t, run.db, play.limp)
	assertR2PassedOnSettledFeatures(t, run.db, planned.settledSteps())
	grade, output := c.outcome(t, runDir)
	story.assertTheOutcome(t, twinOutcome{db: run.db, runDir: runDir, ran: ran, grade: grade, output: output})
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	c.scoreVerdict(t, run.db, runDir, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)},
		campaign.Verdict{Version: "belt-m4", Arm: "master-loss", Subjects: planned.subjects(c.subjects(t, play.story.Plant, planned.faulted, planned.faults()))})
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
	run.stop()
	assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, exportDevice(planned.device)...)
}

func assertThePromotedFollowersAreSaved(t *testing.T, grade outcomeGrade, output []byte) {
	t.Helper()
	if !grade.Valid {
		t.Fatalf("outcome %s: want a valid story", output)
	}
	assertBeatsEveryBaseline(t, grade)
	asRun, doNothing := grade.run(t, "as_run"), grade.run(t, "do_nothing").trips()
	for trip, times := range asRun.trips() {
		if times > doNothing[trip] {
			t.Errorf("as run %s tripped %d times, doing nothing %d: the promotion's ref_held latch is the plant's, any other trip harms", trip, times, doNothing[trip])
		}
	}
	for _, entity := range asRun.Facts.Entities {
		if slices.Contains(entity.Boundaries, outcomeBound{Name: "stranded"}) {
			t.Errorf("as run %s was stranded: the limp feed must let the survivors restart it after the downstream stop", entity.EntityID)
		}
	}
}

func assertNoRestartWhileLoaded(t *testing.T, grade outcomeGrade, output []byte) {
	t.Helper()
	if !grade.Valid || !grade.Easy || !slices.Contains(grade.Verdicts, outcomeVerdict{Baseline: "do_nothing", Delta: 0, Verdict: "ties"}) {
		t.Fatalf("outcome %s: want a valid, easy story whose run ties doing nothing exactly", output)
	}
	asRun, doNothing := grade.run(t, "as_run"), grade.run(t, "do_nothing")
	for _, call := range asRun.Facts.Calls {
		if strings.Contains(call.Effector, "__start_") || strings.Contains(call.Effector, "__reset_trip") {
			t.Errorf("as run, the line called %s on the loaded belt: want no restart", call.Effector)
		}
	}
	if got, want := asRun.trips(), doNothing.trips(); !maps.Equal(got, want) {
		t.Fatalf("as run the trips are %v, doing nothing %v: a restart of the loaded belt stalls it again", got, want)
	}
}

func (g outcomeGrade) run(t *testing.T, mode string) outcomeRun {
	t.Helper()
	at := slices.IndexFunc(g.Runs, func(run outcomeRun) bool { return run.Mode == mode })
	if at < 0 {
		t.Fatalf("the outcome has no %s run", mode)
	}
	return g.Runs[at]
}

func (r outcomeRun) trips() map[string]int {
	trips := map[string]int{}
	for _, entity := range r.Facts.Entities {
		for _, trip := range entity.Trips {
			trips[entity.EntityID+" "+trip.State]++
		}
	}
	return trips
}
