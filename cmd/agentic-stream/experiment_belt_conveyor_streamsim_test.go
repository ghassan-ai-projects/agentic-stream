package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

const (
	conveyorTripAtS      = 30
	conveyorStopAtS      = 180
	conveyorStopForS     = 60
	conveyorHorizonS     = 360
	conveyorReactionTime = 98 * time.Second
)

var (
	conveyorSeeds = []int64{144108226, 1816007955, 1936376588}
	lostDriveLimp = twinStory{spec: conveyorSpec, routes: []string{"reduce_feed"}, readBacks: [][3]string{{"set_feed", "feed_setpoint", conveyorSetpointLimp}},
		unharmed: []string{"D-1", "D-2"}, diagnosis: "drive_tripped", fault: "conveyor_drive__vfd_fault", faulted: "D-3"}
)

func TestTheLimpFeedSavesTheSurvivorsOfALostDriveOnAPacedStreamsimTwin(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	for _, seed := range conveyorSeeds {
		t.Run("seed-"+strconv.FormatInt(seed, 10), func(t *testing.T) {
			t.Parallel()
			run := startConveyorServe(t)
			runDir := twin.connect(t, "seed "+strconv.FormatInt(seed, 10), twin.plan(t, seed, run.socket, conveyorPolicyDigest(t, conveyorSpec)))
			t.Cleanup(func() { logCommandOutcomes(t, run.db) })
			lostDriveLimp.assertReadBacks(t, run.db, conveyorSetpointLimp)
			ran := readTwinRun(t, runDir)
			ran.assertSeed(t, seed)
			assertLimpFeedInTime(t, run.db, ran.onsetOf(t, lostDriveLimp.faulted))
			twin.assertTheLineIsSaved(t, runDir, lostDriveLimp.unharmed...)
			window := campaign.Window{From: ran.start(), Until: ran.start().Add(conveyorHorizonS * time.Second)}
			twin.scoreVerdict(t, run.db, runDir, window, campaign.Verdict{Version: "belt-m2", Arm: "limp-feed",
				Subjects: twin.subjects(t, "b", lostDriveLimp.faulted, lostDriveLimp.faults())})
			if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
				t.Fatalf("%d events quarantined, want none", quarantined)
			}
			run.stop()
			assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db})
		})
	}
}

func (c conveyorTwin) plan(t *testing.T, seed int64, socket, policyDigest string) string {
	t.Helper()
	return c.sessionPlan(t, "s2-b", socket, policyDigest, func(plan map[string]any) {
		disturbances := plan["disturbances"].([]any)
		disturbances[0].(map[string]any)["at_s"] = conveyorTripAtS
		stop := disturbances[1].(map[string]any)
		stop["at_s"], stop["duration_s"] = conveyorStopAtS, conveyorStopForS
		plan["seed"], plan["horizon_s"] = seed, conveyorHorizonS
	})
}

func assertLimpFeedInTime(t *testing.T, dbPath string, trip time.Time) {
	t.Helper()
	var first string
	query := `SELECT COALESCE(MIN(event_time), '') FROM event_log WHERE event_type = 'conveyor.feed_setpoint.observed'
		AND CAST(json_extract(CAST(payload_json AS TEXT), '$.percent') AS REAL) BETWEEN 17.95 AND 18.05`
	if err := openReadOnly(t, dbPath).QueryRowContext(t.Context(), query).Scan(&first); err != nil {
		t.Fatal(err)
	}
	read, err := kernel.ParseTime(first)
	if err != nil {
		t.Fatalf("no feed setpoint reading of 18 %% in the event log (%q): %v", first, err)
	}
	delay := read.Sub(trip)
	t.Logf("the feed setpoint read 18 %% %s after D-3's trip", delay)
	if delay <= 0 || delay > conveyorReactionTime {
		t.Fatalf("the feed setpoint read 18 %% %s after D-3's trip, want within %s", delay, conveyorReactionTime)
	}
}
