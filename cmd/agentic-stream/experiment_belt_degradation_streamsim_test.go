package main

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

const (
	barrierSliceS        = "60"
	barrierCapMS         = "240000"
	approvalPollInterval = 500 * time.Millisecond
)

type twinStory struct {
	routes    []string
	readBacks [][3]string
	unharmed  []string
	device    string
	diagnosis string
	fault     string
	faulted   string
}

var twinStories = map[string]twinStory{
	"s1-a": {routes: []string{"reduce_feed", "stop_feed", "isolate_drive", "reduce_feed"}, readBacks: isolationRun.readBacks,
		unharmed: []string{"D-1", "D-2", "D-3"}, device: "D-3", diagnosis: "bearing", fault: "conveyor_drive__lube_loss", faulted: "D-3"},
	"s3-a": {routes: []string{"reduce_feed", "stop_feed", "stop_belt"}, readBacks: beltStopRun.readBacks,
		unharmed: []string{"D-1", "D-2"}, device: "B-1", diagnosis: "gearbox_seizing", fault: "conveyor_drive__gear_oil_loss", faulted: "D-3"},
	"s4-a": {faulted: "F-1"},
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestTheDriveDegradationLoopPlaysItsStoriesOnABarrierPacedStreamsimTwin(t *testing.T) {
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	for _, name := range slices.Sorted(maps.Keys(twinStories)) {
		t.Run(name, func(t *testing.T) { twin.playBarrierPaced(t, name, twinStories[name]) })
	}
}

func (c conveyorTwin) playBarrierPaced(t *testing.T, name string, story twinStory) {
	t.Helper()
	run := startDegradationServe(t, degradationSpec, shippedCoverage)
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	plan := c.sessionPlan(t, name, run.socket, conveyorPolicyDigest(t, degradationSpec), func(plan map[string]any) { delete(plan, "pace") })
	stopApproving := approveWhileRunning(t, run)
	runDir := c.connect(t, name, plan, "--slice-s", barrierSliceS, "--barrier-cap-ms", barrierCapMS)
	stopApproving()
	ran := readTwinRun(t, runDir)
	logPhasesSince(t, run.db, ran.onsetOf(t, story.faulted))
	assertRoutes(t, run.db, story.routes)
	for _, readBack := range story.readBacks {
		assertTwinReadBack(t, run.db, readBack[0], readBack[1], readBack[2])
	}
	assertR2PassedOnSettledFeatures(t, run.db, len(slices.DeleteFunc(slices.Clone(story.routes), func(route string) bool { return route == "reduce_feed" })))
	if len(story.routes) > 0 {
		c.assertTheLineIsSaved(t, runDir, story.unharmed...)
	}
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	c.scoreVerdict(t, run.db, runDir, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)},
		campaign.Verdict{Version: "belt-m3", Arm: "planned-stop", Subjects: c.subjects(t, story.diagnosis, story.fault)})
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

func assertRoutes(t *testing.T, dbPath string, want []string) {
	t.Helper()
	if routes := columnValues(t, dbPath, "SELECT effector_route FROM commands ORDER BY created_at, command_id"); !slices.Equal(routes, want) {
		t.Fatalf("commands = %v, want %v", routes, want)
	}
}
