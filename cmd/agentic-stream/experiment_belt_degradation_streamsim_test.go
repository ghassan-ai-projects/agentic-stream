package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

const (
	barrierSliceS        = "60"
	barrierCapMS         = "240000"
	approvalPollInterval = 500 * time.Millisecond
)

type twinStory struct {
	story     string
	spec      string
	steps     map[string]proposal
	routes    []string
	readBacks [][3]string
	unharmed  []string
	device    string
	diagnosis string
	fault     string
	faulted   string
	prognosis string
	lost      lostReading
}

var (
	isolationReadBacks = [][3]string{{"set_feed", "feed_setpoint", plantLimp}, {"set_feed", "feed_setpoint", "0"}, {"stop_drive", "run_command", "0"}}
	bearingIsolation   = twinStory{routes: []string{"reduce_feed", "stop_feed", "isolate_drive", "reduce_feed"}, readBacks: isolationReadBacks,
		unharmed: []string{"D-1", "D-2", "D-3"}, device: "D-3", diagnosis: "bearing", fault: "conveyor_drive__lube_loss", faulted: "D-3"}
	plannedBearingStop = twinStory{spec: plannedStopSpec, steps: plannedStopSteps, routes: []string{"stop_feed", "isolate_drive", "reduce_feed"},
		readBacks: [][3]string{{"set_feed", "feed_setpoint", "0"}, {"stop_drive", "run_command", "0"}, {"set_feed", "feed_setpoint", plantLimp}},
		unharmed:  bearingIsolation.unharmed, device: "D-3", diagnosis: "bearing", fault: bearingIsolation.fault, faulted: "D-3",
		prognosis: "drive3_winding_to_trip_s"}
	plannedStopUnmoved = twinStory{spec: plannedStopSpec, steps: plannedStopSteps, faulted: "F-1"}
	twinStories        = map[string]twinStory{
		"s1-a": bearingIsolation,
		"s1-b": plannedBearingStop,
		"s3-a": {routes: []string{"reduce_feed", "stop_feed", "stop_belt"}, readBacks: beltStopRun.readBacks,
			unharmed: []string{"D-1", "D-2"}, device: "B-1", diagnosis: "gearbox_seizing", fault: "conveyor_drive__gear_oil_loss", faulted: "D-3"},
		"s5-a":              bearingIsolation.withLost(stuckVibration),
		"s5b-a":             bearingIsolation.withLost(droppedVibration),
		"s4-a":              {faulted: "F-1"},
		"s4-b":              plannedStopUnmoved,
		"s4-a-planned-stop": plannedStopUnmoved.of("s4-a"),
		"s0-a-planned-stop": plannedStopUnmoved.of("s0-a"),
	}
)

func (s twinStory) of(story string) twinStory {
	s.story = story
	return s
}

func storyOf(name string) string { return cmp.Or(twinStories[name].story, name) }

func (s twinStory) specPath() string { return cmp.Or(s.spec, degradationSpec) }

func (s twinStory) reasoner() workerfake.ExecuteFunc {
	if s.steps == nil {
		return reasonAboutTheLine(shippedCoverage)
	}
	return takeTheSteps(shippedCoverage, s.steps)
}

type twinPlay struct {
	name  string
	story tableStory
	row   map[string]any
	limp  string
}

func (p twinPlay) seed() int64 { return int64(p.row["seed"].(float64)) }

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestTheDriveDegradationLoopPlaysItsStoriesOnABarrierPacedStreamsimTwin(t *testing.T) {
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	table, rows := twin.storyTable(t), twinRows(t)
	for _, name := range slices.Sorted(maps.Keys(twinStories)) {
		story := table.story(t, storyOf(name))
		limp := twin.limpSetpoint(t, table, story)
		t.Run(name, func(t *testing.T) {
			for i, row := range story.firstRows(rows) {
				play := twinPlay{name: name, story: story, row: row, limp: limp}
				t.Run(fmt.Sprintf("row-%d", i+1), func(t *testing.T) { twin.playBarrierPaced(t, play, twinStories[name]) })
			}
		})
	}
}

func (c conveyorTwin) playBarrierPaced(t *testing.T, play twinPlay, story twinStory) {
	t.Helper()
	started := time.Now()
	t.Cleanup(func() {
		t.Logf("%s seed %d played in %s of wall time", play.name, play.seed(), time.Since(started).Round(time.Second))
	})
	run := startDegradationServe(t, story.specPath(), play.story.Plant, story.reasoner())
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	plan := c.sessionPlan(t, play.story.ID, run.socket, conveyorPolicyDigest(t, story.specPath()), play.story.planOf(play.row))
	stopApproving := approveWhileRunning(t, run)
	runDir := c.connect(t, play.name, plan, "--slice-s", barrierSliceS, "--barrier-cap-ms", barrierCapMS)
	stopApproving()
	ran := readTwinRun(t, runDir)
	ran.assertSeed(t, play.seed())
	onset := ran.onsetOf(t, story.faulted)
	logPhasesSince(t, run.db, onset)
	logCommandsSince(t, run.db, onset)
	assertRoutes(t, run.db, story.routes)
	for _, readBack := range story.readBacks {
		if readBack[2] == plantLimp {
			readBack[2] = play.limp
		}
		assertTwinReadBack(t, run.db, readBack[0], readBack[1], readBack[2])
	}
	assertR2PassedOnSettledFeatures(t, run.db, len(slices.DeleteFunc(slices.Clone(story.routes), func(route string) bool { return route == "reduce_feed" })))
	if len(story.routes) > 0 {
		story.judge(t, c.assertTheLineIsSaved(t, runDir, story.unharmed...), ran)
	}
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	c.scoreVerdict(t, run.db, runDir, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)},
		campaign.Verdict{Version: "belt-m4", Arm: "planned-stop", Subjects: story.subjects(c.subjects(t, play.story.Plant, "D-3", story.faults()))})
	story.lost.assertReportedBeforeTheCause(t, runDir, story.fault)
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
	run.stop()
	assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, exportDevice(story.device)...)
}

func exportDevice(device string) []string {
	if device == "" {
		return nil
	}
	return []string{"--device-id", device}
}

func approveWhileRunning(t *testing.T, run degradationServe) func() {
	t.Helper()
	desk := approvalDesk(run.address, conveyorApprovalToken, conveyorApprover, campaign.ApproveAll{})
	db := openReadOnly(t, run.db)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(approvalPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := desk.Act(ctx, db, time.Now()); err != nil && ctx.Err() == nil {
					t.Errorf("answer the pending approvals: %v", err)
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}

func logPhasesSince(t *testing.T, dbPath string, onset time.Time) {
	t.Helper()
	rows, err := openReadOnly(t, dbPath).QueryContext(t.Context(), "SELECT version, phase, event_horizon, completeness FROM situation_versions ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var version int
		var phase, horizon, completeness string
		if err := rows.Scan(&version, &phase, &horizon, &completeness); err != nil {
			t.Fatal(err)
		}
		at, err := kernel.ParseTime(horizon)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("v%d %s (%s) at onset %+.2f h", version, phase, completeness, at.Sub(onset).Hours())
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func logCommandsSince(t *testing.T, dbPath string, onset time.Time) {
	t.Helper()
	rows, err := openReadOnly(t, dbPath).QueryContext(t.Context(), `SELECT c.effector_route, COALESCE(json_extract(CAST(c.command_json AS TEXT), '$.payload.level'), json_extract(CAST(c.command_json AS TEXT), '$.payload.mode')), c.status, v.phase, v.event_horizon
		FROM commands c JOIN intents i ON i.intent_id = c.intent_id
		JOIN situation_versions v ON v.situation_id = i.situation_id AND v.version = i.situation_version ORDER BY c.created_at, c.command_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var route, command, status, phase, horizon string
		if err := rows.Scan(&route, &command, &status, &phase, &horizon); err != nil {
			t.Fatal(err)
		}
		at, err := kernel.ParseTime(horizon)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("command %s(%s) %s on %s at onset %+.2f h", route, command, status, phase, at.Sub(onset).Hours())
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func assertRoutes(t *testing.T, dbPath string, want []string) {
	t.Helper()
	query := "SELECT effector_route FROM commands WHERE effector_route != 'install_watch_condition' ORDER BY created_at, command_id"
	if routes := columnValues(t, dbPath, query); !slices.Equal(routes, want) {
		t.Fatalf("the line's commands = %v, want %v (a watch is no step)", routes, want)
	}
}
