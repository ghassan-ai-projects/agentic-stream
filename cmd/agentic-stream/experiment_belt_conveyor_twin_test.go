package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

type conveyorTwin struct{ binary, repository string }

func (c conveyorTwin) examples() string { return filepath.Join(c.repository, "examples") }

func (c conveyorTwin) adapters() string { return filepath.Join(c.repository, "adapters") }

func (c conveyorTwin) kinds() string { return filepath.Join(c.examples(), "belt-conveyor", "kinds") }

func (c conveyorTwin) connect(t *testing.T, session, plan string, flags ...string) string {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "run")
	args := append([]string{c.binary, "plantwire", "connect", "--plan", plan, "--domains-dir", c.examples(), "--adapters-dir", c.adapters(),
		"--kinds-dir", c.kinds(), "--out", runDir}, flags...)
	node := exec.Cmd{Path: c.binary, Args: args}
	runUntilDeadline(t, session, &node)
	return runDir
}

func (c conveyorTwin) sessionPlan(t *testing.T, story, socket, policyDigest string, edit func(plan map[string]any)) string {
	t.Helper()
	plan := readJSONFile(t, filepath.Join(c.examples(), "belt-conveyor", "sessions", story+".session-plan.json"))
	plan["host_socket"], plan["policies"] = socket, []string{policyDigest}
	edit(plan)
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), story+".session-plan.json")
	writePrivateFile(t, path, string(raw))
	return path
}

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
			Effector string `json:"effector"`
			AtNS     int64  `json:"at_ns"`
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

type outcomeGrade struct {
	Valid    bool             `json:"valid"`
	Easy     bool             `json:"easy"`
	Runs     []outcomeRun     `json:"runs"`
	Verdicts []outcomeVerdict `json:"verdicts"`
}

type outcomeRun struct {
	Mode  string `json:"mode"`
	Facts struct {
		Entities []outcomeEntity `json:"entities"`
		Calls    []struct {
			Effector string `json:"effector"`
		} `json:"calls"`
	} `json:"facts"`
}

type outcomeEntity struct {
	EntityID   string         `json:"entity_id"`
	Trips      []outcomeTrip  `json:"trips"`
	Boundaries []outcomeBound `json:"boundaries"`
}

type outcomeTrip struct {
	State   string `json:"state"`
	FirstNS int64  `json:"first_ns"`
}

type outcomeBound struct {
	Name string `json:"name"`
}

type outcomeVerdict struct {
	Baseline string  `json:"baseline"`
	Delta    float64 `json:"delta"`
	Verdict  string  `json:"verdict"`
}

func (c conveyorTwin) assertTheLineIsSaved(t *testing.T, runDir string, unharmed ...string) outcomeGrade {
	t.Helper()
	grade, output := c.outcome(t, runDir)
	assertTheOutcomeSavesTheLine(t, grade, output, unharmed)
	return grade
}

func assertTheOutcomeSavesTheLine(t *testing.T, grade outcomeGrade, output []byte, unharmed []string) {
	t.Helper()
	if !grade.Valid || !slices.ContainsFunc(grade.Verdicts, func(v outcomeVerdict) bool { return v.Baseline == "do_nothing" && v.Verdict == "beats" }) {
		t.Fatalf("outcome %s: want a valid story whose run beats doing nothing", output)
	}
	assertNoTripNorStrandedBelt(t, grade, output, unharmed)
}

func (c conveyorTwin) outcome(t *testing.T, runDir string) (outcomeGrade, []byte) {
	t.Helper()
	output := c.run(t, "outcome", "--run", runDir, "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	var grade outcomeGrade
	if err := json.Unmarshal(output, &grade); err != nil {
		t.Fatalf("decode the outcome %s: %v", output, err)
	}
	t.Logf("outcome valid %t, easy %t, verdicts %+v", grade.Valid, grade.Easy, grade.Verdicts)
	return grade, output
}

func assertNoTripNorStrandedBelt(t *testing.T, grade outcomeGrade, output []byte, unharmed []string) {
	t.Helper()
	asRun := slices.IndexFunc(grade.Runs, func(run outcomeRun) bool { return run.Mode == "as_run" })
	if asRun < 0 {
		t.Fatalf("outcome %s has no as_run", output)
	}
	for _, entity := range grade.Runs[asRun].Facts.Entities {
		tripped := slices.Contains(unharmed, entity.EntityID) && len(entity.Trips) > 0
		if tripped || slices.Contains(entity.Boundaries, outcomeBound{Name: "stranded"}) {
			t.Fatalf("as run, %s has trips %v and boundaries %v: want no trip of %v and no stranded belt\n%s", entity.EntityID, entity.Trips, entity.Boundaries, unharmed, output)
		}
	}
}

func (c conveyorTwin) scoreVerdict(t *testing.T, dbPath, runDir string, window campaign.Window, verdict campaign.Verdict) {
	t.Helper()
	if err := campaign.WriteVerdictOf(t.Context(), openReadOnly(t, dbPath), runDir, window, verdict); err != nil {
		t.Fatal(err)
	}
	output := c.run(t, "score", "--run", filepath.Join(runDir, "run.json"), "--domains-dir", c.examples(), "--adapters-dir", c.adapters())
	t.Logf("score:\n%s", output)
}

func TestTheTwinNamesEachActionByTheRawCommandOfItsDevicesKind(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	subjects := conveyorTwin{binary: binary, repository: repository}.subjects(t, "a", "D-3", nil)
	for route, want := range map[string][2]string{"isolate_drive": {"D-3", "conveyor_drive__stop_drive"}, "reduce_feed": {"F-1", "bulk_feeder__set_feed"}} {
		if device, raw := subjects.Acted(route, "conveyor-1"); device != want[0] || raw != want[1] {
			t.Errorf("%s acts on %s with %s, want %v: a role kind's board names the same command", route, device, raw, want)
		}
	}
}

func (c conveyorTwin) subjects(t *testing.T, plant, faulted string, faults map[string]string) campaign.Subjects {
	t.Helper()
	routes, kinds, raw := readConveyorRoutes(t, plant), siteKinds(t, conveyorSiteOf(plant)), c.rawCommands(t)
	return campaign.Subjects{
		Detected: func(entityID, diagnosed string) (string, string) {
			if fault, known := faults[diagnosed]; known {
				return faulted, fault
			}
			switch diagnosed {
			case untoldCause:
				return faulted, ""
			case spec.UnknownDiagnosis:
				return "", ""
			}
			return entityID, diagnosed
		},
		Acted: func(route, target string) (string, string) {
			device := routes[route].TargetBindings[target]
			return device, raw[kinds[device]][routes[route].Operation]
		},
		Refines: causesRefiningTheSymptom,
	}
}

type conveyorRoute struct {
	Operation      string                        `json:"operation"`
	TargetBindings map[string]string             `json:"target_bindings"`
	Presets        map[string]map[string]float64 `json:"presets"`
}

func readConveyorRoutes(t *testing.T, plant string) map[string]conveyorRoute {
	t.Helper()
	var catalog struct {
		Routes map[string]conveyorRoute `json:"routes"`
	}
	decodeFile(t, conveyorCatalogOf(plant), &catalog)
	return catalog.Routes
}

func (c conveyorTwin) rawCommands(t *testing.T) map[string]map[string]string {
	t.Helper()
	boards, err := filepath.Glob(filepath.Join(c.kinds(), "*.board.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]map[string]string{}
	for _, path := range boards {
		var board struct {
			Kinds map[string]struct {
				Commands map[string]struct {
					Raw string `json:"raw"`
				} `json:"commands"`
			} `json:"kinds"`
		}
		decodeFile(t, path, &board)
		for name, kind := range board.Kinds {
			raw[name] = map[string]string{}
			for command, mapped := range kind.Commands {
				raw[name][command] = mapped.Raw
			}
		}
	}
	return raw
}

func siteKinds(t *testing.T, sitePath string) map[string]string {
	t.Helper()
	var site struct {
		Devices map[string]struct {
			Kind string `json:"kind"`
		} `json:"devices"`
	}
	decodeFile(t, sitePath, &site)
	kinds := map[string]string{}
	for device, declared := range site.Devices {
		kinds[device] = declared.Kind
	}
	return kinds
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
