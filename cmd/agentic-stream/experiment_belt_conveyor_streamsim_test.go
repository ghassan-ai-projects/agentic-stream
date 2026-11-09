package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

var conveyorSeeds = []int64{144108226, 1816007955, 1936376588}

type conveyorTwin struct{ binary, repository string }

func TestTheLimpFeedSavesTheSurvivorsOfALostDriveOnAPacedStreamsimTwin(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	twin := conveyorTwin{binary: binary, repository: repository}
	for _, seed := range conveyorSeeds {
		t.Run("seed-"+strconv.FormatInt(seed, 10), func(t *testing.T) {
			t.Parallel()
			run := startConveyorServe(t)
			runDir := twin.session(t, seed, run.socket)
			t.Cleanup(func() { logCommandOutcomes(t, run.db) })
			assertTwinReadBack(t, run.db, "set_feed", "feed_setpoint", conveyorSetpointLimp)
			ran := readTwinRun(t, runDir)
			ran.assertSeed(t, seed)
			assertLimpFeedInTime(t, run.db, ran.onsetOf(t, "D-3"))
			twin.assertTheSurvivorsAreSaved(t, runDir)
			twin.scoreVerdict(t, run.db, runDir, ran.start())
			if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
				t.Fatalf("%d events quarantined, want none", quarantined)
			}
			run.stop()
			assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db})
		})
	}
}

func (c conveyorTwin) session(t *testing.T, seed int64, socket string) string {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "run")
	node := exec.Cmd{Path: c.binary, Args: []string{c.binary, "plantwire", "connect", "--plan", c.plan(t, seed, socket),
		"--domains-dir", c.examples(), "--adapters-dir", c.adapters(), "--kinds-dir", c.kinds(), "--out", runDir}}
	runUntilDeadline(t, "seed "+strconv.FormatInt(seed, 10), &node)
	return runDir
}

func (c conveyorTwin) plan(t *testing.T, seed int64, socket string) string {
	t.Helper()
	plan := readJSONFile(t, filepath.Join(c.examples(), "belt-conveyor", "sessions", "s2-b.session-plan.json"))
	disturbances := plan["disturbances"].([]any)
	disturbances[0].(map[string]any)["at_s"] = conveyorTripAtS
	stop := disturbances[1].(map[string]any)
	stop["at_s"], stop["duration_s"] = conveyorStopAtS, conveyorStopForS
	plan["seed"], plan["horizon_s"], plan["host_socket"], plan["policies"] = seed, conveyorHorizonS, socket, []string{conveyorPolicyDigest(t)}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "s2-b.session-plan.json")
	writePrivateFile(t, path, string(raw))
	return path
}

func (c conveyorTwin) examples() string { return filepath.Join(c.repository, "examples") }

func (c conveyorTwin) adapters() string { return filepath.Join(c.repository, "adapters") }

func (c conveyorTwin) kinds() string { return filepath.Join(c.examples(), "belt-conveyor", "kinds") }

type twinRun struct {
	RunID               string `json:"run_id"`
	Seed                int64  `json:"seed"`
	ExpectedTraceDigest string `json:"expected_trace_digest"`
	WorldConfig         struct {
		StartTimeNS int64 `json:"start_time_ns"`
	} `json:"world_config"`
	CommandLog []struct {
		Op   string `json:"op"`
		Args struct {
			EntityID string `json:"entity_id"`
			OnsetNS  int64  `json:"onset_ns"`
		} `json:"args"`
	} `json:"command_log"`
}

func readTwinRun(t *testing.T, runDir string) twinRun {
	t.Helper()
	var run twinRun
	decodeFile(t, filepath.Join(runDir, "run.json"), &run)
	return run
}

func (r twinRun) assertSeed(t *testing.T, seed int64) {
	t.Helper()
	t.Logf("run %s: seed %d, trace %s", r.RunID, r.Seed, r.ExpectedTraceDigest)
	if r.Seed != seed {
		t.Fatalf("the run's seed is %d, want the plan's %d", r.Seed, seed)
	}
}

func (r twinRun) start() time.Time { return time.Unix(0, r.WorldConfig.StartTimeNS).UTC() }

func (r twinRun) onsetOf(t *testing.T, entity string) time.Time {
	t.Helper()
	for _, command := range r.CommandLog {
		if command.Op == "fault.inject" && command.Args.EntityID == entity {
			return time.Unix(0, command.Args.OnsetNS).UTC()
		}
	}
	t.Fatalf("the run injected no fault into %s", entity)
	return time.Time{}
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

type outcomeGrade struct {
	Valid    bool             `json:"valid"`
	Runs     []outcomeRun     `json:"runs"`
	Verdicts []outcomeVerdict `json:"verdicts"`
}

type outcomeRun struct {
	Mode  string `json:"mode"`
	Facts struct {
		Entities []outcomeEntity `json:"entities"`
	} `json:"facts"`
}

type outcomeEntity struct {
	EntityID   string         `json:"entity_id"`
	Trips      []outcomeTrip  `json:"trips"`
	Boundaries []outcomeBound `json:"boundaries"`
}

type outcomeTrip struct {
	State string `json:"state"`
}

type outcomeBound struct {
	Name string `json:"name"`
}

type outcomeVerdict struct {
	Baseline string  `json:"baseline"`
	Delta    float64 `json:"delta"`
	Verdict  string  `json:"verdict"`
}

func (c conveyorTwin) assertTheSurvivorsAreSaved(t *testing.T, runDir string) {
	t.Helper()
	output := c.run(t, "outcome", "--run", runDir, "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	var grade outcomeGrade
	if err := json.Unmarshal(output, &grade); err != nil {
		t.Fatalf("decode the outcome %s: %v", output, err)
	}
	t.Logf("outcome verdicts %+v", grade.Verdicts)
	if !grade.Valid || !slices.ContainsFunc(grade.Verdicts, func(v outcomeVerdict) bool { return v.Baseline == "do_nothing" && v.Verdict == "beats" }) {
		t.Fatalf("outcome %s: want a valid story whose run beats doing nothing", output)
	}
	assertNoSurvivorTripNorStrandedBelt(t, grade, output)
}

func assertNoSurvivorTripNorStrandedBelt(t *testing.T, grade outcomeGrade, output []byte) {
	t.Helper()
	asRun := slices.IndexFunc(grade.Runs, func(run outcomeRun) bool { return run.Mode == "as_run" })
	if asRun < 0 {
		t.Fatalf("outcome %s has no as_run", output)
	}
	for _, entity := range grade.Runs[asRun].Facts.Entities {
		survivor := entity.EntityID == "D-1" || entity.EntityID == "D-2"
		stranded := slices.Contains(entity.Boundaries, outcomeBound{Name: "stranded"})
		if survivor && len(entity.Trips) > 0 || stranded {
			t.Fatalf("as run, %s has trips %v and boundaries %v: want no survivor trip and no stranded belt\n%s", entity.EntityID, entity.Trips, entity.Boundaries, output)
		}
	}
}

func (c conveyorTwin) scoreVerdict(t *testing.T, dbPath, runDir string, start time.Time) {
	t.Helper()
	window := campaign.Window{From: start, Until: start.Add(conveyorHorizonS * time.Second)}
	verdict := campaign.Verdict{Version: "belt-m2", Arm: "limp-feed", Subjects: c.subjects(t)}
	if err := campaign.WriteVerdictOf(t.Context(), openReadOnly(t, dbPath), runDir, window, verdict); err != nil {
		t.Fatal(err)
	}
	output := c.run(t, "score", "--run", filepath.Join(runDir, "run.json"), "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	t.Logf("score:\n%s", output)
}

func (c conveyorTwin) subjects(t *testing.T) campaign.Subjects {
	t.Helper()
	var catalog struct {
		Routes map[string]struct {
			Operation      string            `json:"operation"`
			TargetBindings map[string]string `json:"target_bindings"`
		} `json:"routes"`
	}
	decodeFile(t, conveyorCatalog, &catalog)
	raw := c.rawCommands(t)
	return campaign.Subjects{
		Detected: func(entityID, diagnosis string) (string, string) {
			if diagnosis == "drive_tripped" {
				return "D-3", "conveyor_drive__vfd_fault"
			}
			return entityID, diagnosis
		},
		Acted: func(route, target string) (string, string) {
			return catalog.Routes[route].TargetBindings[target], raw[catalog.Routes[route].Operation]
		},
	}
}

func (c conveyorTwin) rawCommands(t *testing.T) map[string]string {
	t.Helper()
	var board struct {
		Kinds map[string]struct {
			Commands map[string]struct {
				Raw string `json:"raw"`
			} `json:"commands"`
		} `json:"kinds"`
	}
	decodeFile(t, filepath.Join(c.kinds(), "feeder-controller.board.json"), &board)
	raw := map[string]string{}
	for _, kind := range board.Kinds {
		for name, command := range kind.Commands {
			raw[name] = command.Raw
		}
	}
	return raw
}

func (c conveyorTwin) run(t *testing.T, args ...string) []byte {
	t.Helper()
	command := exec.Cmd{Path: c.binary, Args: append([]string{c.binary}, args...)}
	output, err := command.Output()
	if err != nil {
		t.Fatalf("streamsim %v: %v\n%s", args, err, output)
	}
	return output
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	var document map[string]any
	decodeFile(t, path, &document)
	return document
}

func decodeFile(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
