package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

const liveBarrierCapMS = "330000"

var liveBudget = map[string]string{"wallTime: 30s": "wallTime: 300s", "inputTokens: 16000": "inputTokens: 24000",
	"outputTokens: 2000": "outputTokens: 20000", "costMicrounits: 400000": "costMicrounits: 800000"}

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
	story, table := liveStories[name], c.storyTable(t)
	played := story.inTable(t, table, name)
	limp := c.limpSetpoint(t, table, played)
	out := live.runFolder(t, fmt.Sprintf("%s-row%d", name, row))
	spec := editedSpec(t, story.planned.specPath(), out, liveBudget)
	meter, workerSocket := serveMeteredTamoz(t, out, live.startTamoz(t, out))
	run := startLiveServe(t, spec, story.plant, out, workerSocket)
	t.Cleanup(func() { logCommandOutcomes(t, run.db) })
	seed, plan := c.livePlan(t, story, played, row, run.socket, conveyorPolicyDigest(t, spec))
	stopApproving := approveWhileRunning(t, run)
	started := time.Now()
	runDir := c.connect(t, fmt.Sprintf("%s row %d", name, row), plan, story.pacing()...)
	stopApproving()
	meter.report(t, out, time.Since(started))
	ran := readTwinRun(t, runDir)
	ran.assertSeed(t, seed)
	onset := ran.onsetOf(t, story.planned.faulted)
	logPhasesSince(t, run.db, onset)
	logCommandsSince(t, run.db, onset)
	horizon := time.Duration(readJSONFile(t, plan)["horizon_s"].(float64) * float64(time.Second))
	output := c.keepEvidence(t, runDir, out, campaign.Window{From: ran.start(), Until: ran.start().Add(horizon)}, story.subjects(t, c))
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Errorf("%d events quarantined, want none", quarantined)
	}
	run.stop()
	assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, exportDevice(story.planned.device)...)
	story.assertPlayed(t, twinOutcome{db: run.db, runDir: runDir, ran: ran, grade: decodeOutcome(t, output), output: output}, onset, limp)
}

func startLiveServe(t *testing.T, specPath, plant, out, workerSocket string) degradationServe {
	t.Helper()
	dir := privateSocketDir(t)
	run := degradationServe{plantWireServe: plantWireServe{dir: out, socket: filepath.Join(dir, "host.sock"), db: filepath.Join(out, "stream.db")},
		address: freeLoopbackAddress(t)}
	seedMigratedDatabase(t, run.db)
	applyPrincipalsFile(t, run.db, conveyorPrincipals)
	run.stop = servePlantWire(t, run.plantWireServe, run.address, specPath, conveyorKinds, conveyorSiteOf(plant), "--device-catalog", conveyorCatalogOf(plant),
		"--dispatch-lease", conveyorRouteLease, "--worker-socket", workerSocket, "--worker-name", "tamoz", "--event-schemas", conveyorEventSchemas)
	return run
}

func (s liveStory) pacing() []string {
	if !s.barrier {
		return nil
	}
	return []string{"--slice-s", barrierSliceS, "--barrier-cap-ms", liveBarrierCapMS}
}

func (c conveyorTwin) livePlan(t *testing.T, story liveStory, played tableStory, row int, socket, policyDigest string) (int64, string) {
	t.Helper()
	values := played.Rows[row-1]
	seed := int64(values["seed"].(float64))
	if !story.barrier {
		return seed, c.plan(t, seed, socket, policyDigest)
	}
	return seed, c.sessionPlan(t, played.ID, socket, policyDigest, played.planOf(values))
}

func (c conveyorTwin) keepEvidence(t *testing.T, runDir, out string, window campaign.Window, subjects campaign.Subjects) []byte {
	t.Helper()
	outcome := c.run(t, "outcome", "--run", runDir, "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	writePrivateFile(t, filepath.Join(out, "outcome.json"), string(outcome))
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
