package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

const liveBarrierCapMS = "330000"

var (
	liveBudget = map[string]string{"wallTime: 30s": "wallTime: 300s", "inputTokens: 16000": "inputTokens: 24000",
		"outputTokens: 2000": "outputTokens: 20000", "costMicrounits: 400000": "costMicrounits: 800000"}
	liveFaults = map[string]string{"bearing": "conveyor_drive__lube_loss", "gearbox_seizing": "conveyor_drive__gear_oil_loss",
		"drive_tripped": "conveyor_drive__vfd_fault"}
	liveStories = map[string]liveStory{
		"s1-a": {spec: degradationSpec, plant: "a", barrier: true, planned: twinStories["s1-a"]},
		"s2-b": {spec: conveyorSpec, plant: "b", planned: twinStory{routes: []string{"reduce_feed"}, readBacks: [][3]string{{"set_feed", "feed_setpoint", conveyorSetpointLimp}},
			unharmed: []string{"D-1", "D-2"}, faulted: "D-3"}},
		"s3-a": {spec: degradationSpec, plant: "a", barrier: true, planned: twinStories["s3-a"]},
	}
)

type liveStory struct {
	spec    string
	plant   string
	barrier bool
	planned twinStory
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv); one model run at a time
func TestTamozReasonsTheConveyorStoriesOnTheStreamsimTwin(t *testing.T) {
	live := liveModelUnderTest(t)
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	for _, name := range slices.Sorted(maps.Keys(liveStories)) {
		t.Run(name, func(t *testing.T) {
			for _, row := range live.rows {
				t.Run(fmt.Sprintf("row-%d", row), func(t *testing.T) { twin.playLive(t, live, name, row) })
			}
		})
	}
}

func (c conveyorTwin) playLive(t *testing.T, live liveModel, name string, row int) {
	t.Helper()
	story := liveStories[name]
	out := live.runFolder(t, fmt.Sprintf("%s-row%d", name, row))
	spec := editedSpec(t, story.spec, out, liveBudget)
	meter, workerSocket := serveMeteredTamoz(t, out, live.startTamoz(t, out))
	run := startLiveServe(t, spec, story.plant, out, workerSocket)
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	seed, plan := c.livePlan(t, story, name, row, run.socket, conveyorPolicyDigest(t, spec))
	stopApproving := approveWhileRunning(t, run)
	started := time.Now()
	runDir := c.connect(t, fmt.Sprintf("%s row %d", name, row), plan, story.pacing()...)
	stopApproving()
	meter.report(t, out, time.Since(started))
	ran := readTwinRun(t, runDir)
	ran.assertSeed(t, seed)
	onset := ran.onsetOf(t, story.planned.faulted)
	logPhasesSince(t, run.db, onset)
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	outcome := c.keepEvidence(t, story.plant, runDir, out, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)})
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Errorf("%d events quarantined, want none", quarantined)
	}
	run.stop()
	assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, exportDevice(story.planned.device)...)
	story.assertPlayed(t, run.db, onset, outcome)
}

func startLiveServe(t *testing.T, specPath, plant, out, workerSocket string) degradationServe {
	t.Helper()
	dir := privateSocketDir(t)
	run := degradationServe{plantWireServe: plantWireServe{dir: out, socket: filepath.Join(dir, "host.sock"), db: filepath.Join(out, "stream.db")},
		address: freeLoopbackAddress(t)}
	seedMigratedDatabase(t, run.db)
	applyPrincipalsFile(t, run.db, conveyorPrincipals)
	run.stop = servePlantWire(t, run.plantWireServe, run.address, specPath, conveyorKinds, conveyorSite, "--device-catalog", conveyorCatalogOf(plant),
		"--dispatch-lease", conveyorRouteLease, "--worker-socket", workerSocket, "--worker-name", "tamoz", "--event-schemas", conveyorEventSchemas)
	return run
}

func (s liveStory) pacing() []string {
	if !s.barrier {
		return nil
	}
	return []string{"--slice-s", barrierSliceS, "--barrier-cap-ms", liveBarrierCapMS}
}

func (c conveyorTwin) livePlan(t *testing.T, story liveStory, name string, row int, socket, policyDigest string) (int64, string) {
	t.Helper()
	declared, values := c.storyRow(t, name, row)
	seed := int64(values["seed"].(float64))
	if !story.barrier {
		return seed, c.plan(t, seed, socket, policyDigest)
	}
	return seed, c.sessionPlan(t, name, socket, policyDigest, func(plan map[string]any) {
		delete(plan, "pace")
		plan["seed"] = seed
		for i, disturbance := range declared["disturbances"].([]any) {
			fields, planned := disturbance.(map[string]any), plan["disturbances"].([]any)[i].(map[string]any)
			planned["at_s"] = values[fields["at"].(string)]
			if severity, ok := fields["severity"].(string); ok {
				planned["severity"] = values[severity]
			}
		}
	})
}

func (c conveyorTwin) storyRow(t *testing.T, name string, row int) (map[string]any, map[string]any) {
	t.Helper()
	var table struct {
		Stories []map[string]any `json:"stories"`
	}
	decodeFile(t, filepath.Join(c.examples(), "belt-conveyor", "stories.json"), &table)
	for _, story := range table.Stories {
		if story["id"] == name {
			return story, story["rows"].([]any)[row-1].(map[string]any)
		}
	}
	t.Fatalf("stories.json has no story %s", name)
	return nil, nil
}

func (c conveyorTwin) keepEvidence(t *testing.T, plant, runDir, out string, window campaign.Window) []byte {
	t.Helper()
	outcome := c.run(t, "outcome", "--run", runDir, "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	writePrivateFile(t, filepath.Join(out, "outcome.json"), string(outcome))
	subjects := c.subjects(t, plant, "D-3", liveFaults)
	db := openReadOnly(t, filepath.Join(out, "stream.db"))
	if err := campaign.WriteVerdictOf(t.Context(), db, runDir, window, campaign.Verdict{Version: "belt-m4-live", Arm: "tamoz", Subjects: subjects}); err != nil {
		t.Fatal(err)
	}
	score := c.run(t, "score", "--run", filepath.Join(runDir, "run.json"), "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	writePrivateFile(t, filepath.Join(out, "score.txt"), string(score))
	t.Logf("score:\n%s", score)
	if err := os.CopyFS(filepath.Join(out, "run"), os.DirFS(runDir)); err != nil {
		t.Fatalf("keep the twin's run folder: %v", err)
	}
	return outcome
}

func (s liveStory) assertPlayed(t *testing.T, dbPath string, onset time.Time, outcome []byte) {
	t.Helper()
	var grade outcomeGrade
	if err := json.Unmarshal(outcome, &grade); err != nil {
		t.Fatalf("decode the outcome: %v", err)
	}
	t.Logf("outcome verdicts %+v", grade.Verdicts)
	if !grade.Valid || !slices.ContainsFunc(grade.Verdicts, func(v outcomeVerdict) bool { return v.Baseline == "do_nothing" && v.Verdict == "beats" }) {
		t.Errorf("the run does not beat doing nothing in a valid story")
	}
	assertRoutes(t, dbPath, s.planned.routes)
	for _, readBack := range s.planned.readBacks {
		if readBack[2] == plantLimp {
			readBack[2] = catalogLimp(t, s.plant)
		}
		assertTwinReadBack(t, dbPath, readBack[0], readBack[1], readBack[2])
	}
	if !s.barrier {
		assertLimpFeedInTime(t, dbPath, onset)
	}
	assertNoTripNorStrandedBelt(t, grade, outcome, s.planned.unharmed)
}

func catalogLimp(t *testing.T, plant string) string {
	t.Helper()
	setpoint := readConveyorRoutes(t, plant)[limpFeed.Type].Presets[limpFeed.Parameters["level"].(string)]["feed_setpoint"]
	return strconv.FormatFloat(setpoint, 'f', -1, 64)
}
