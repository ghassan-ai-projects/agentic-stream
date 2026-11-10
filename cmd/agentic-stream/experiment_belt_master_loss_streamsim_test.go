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
	route     string
	readBack  string
	approvals int
	phases    []string
	judge     func(c conveyorTwin, t *testing.T, runDir string)
}

var masterLossTwins = map[string]masterLossTwin{
	"s6-a-stop": {route: "stop_feed", readBack: "0", approvals: 1, phases: []string{"running", "master_lost", "stranded"},
		judge: conveyorTwin.assertNoRestartWhileLoaded},
	"s6-a-promote": {route: "reduce_feed", readBack: plantLimp, phases: []string{"running", "master_lost", "followers_lead"},
		judge: conveyorTwin.assertThePromotedFollowersAreSaved},
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
	run := startDegradationServe(t, masterLossSpec, play.story.Plant, answerTheMastersTrip(masterLossSteps))
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	plan := c.sessionPlan(t, play.story.ID, run.socket, conveyorPolicyDigest(t, masterLossSpec), play.story.planOf(play.row))
	stopApproving := approveWhileRunning(t, run)
	runDir := c.connect(t, play.name, plan, "--slice-s", barrierSliceS, "--barrier-cap-ms", barrierCapMS)
	stopApproving()
	ran := readTwinRun(t, runDir)
	ran.assertSeed(t, play.seed())
	trip := ran.onsetOf(t, "D-1")
	logPhasesSince(t, run.db, trip)
	logCommandsSince(t, run.db, trip)
	assertPhases(t, run.db, story.phases)
	assertRoutes(t, run.db, []string{story.route})
	assertTwinReadBack(t, run.db, "set_feed", "feed_setpoint", strings.Replace(story.readBack, plantLimp, play.limp, 1))
	assertR2PassedOnSettledFeatures(t, run.db, story.approvals)
	assertEveryStepNames(t, run.db, masterTripped)
	story.judge(c, t, runDir)
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	c.scoreVerdict(t, run.db, runDir, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)},
		campaign.Verdict{Version: "belt-m4", Arm: "master-loss", Subjects: c.subjects(t, play.story.Plant, "D-1", map[string]string{masterTripped: masterFault})})
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
	run.stop()
	assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, "--device-id", "F-1")
}

func (c conveyorTwin) assertThePromotedFollowersAreSaved(t *testing.T, runDir string) {
	t.Helper()
	grade, output := c.outcome(t, runDir)
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

func (c conveyorTwin) assertNoRestartWhileLoaded(t *testing.T, runDir string) {
	t.Helper()
	grade, output := c.outcome(t, runDir)
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
