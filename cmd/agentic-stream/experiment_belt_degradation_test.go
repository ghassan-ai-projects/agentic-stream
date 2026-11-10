package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

const (
	conveyorApprovalToken = "conveyor-approval-token"
	conveyorRelay         = "conveyor-relay"
	conveyorApprover      = "conveyor-approver"
	degradationTicks      = 40
	surgeTicks            = 22
	undecidedCauseWait    = "5h"
)

var (
	degradationSpec    = filepath.Join(conveyorData, "drive-degradation.situation.yaml")
	conveyorPrincipals = filepath.Join(conveyorData, "principals.yaml")
	isolationRun       = plannedRun{
		degrading: untoldCause, story: bearingDegradation, device: "D-3",
		order: []string{"F-1 set_feed map[feed_setpoint:54]", "F-1 set_feed map[feed_setpoint:0]", "D-3 stop_drive map[run_command:0]",
			"F-1 set_feed map[feed_setpoint:54]"},
		readBacks: [][3]string{{"set_feed", "feed_setpoint", "54"}, {"set_feed", "feed_setpoint", "0"}, {"stop_drive", "run_command", "0"}},
		phases:    []string{"normal", "watch", "degrading", "shedding", "cause_bearing", "isolated"},
	}
	beltStopRun = plannedRun{
		degrading: "gearbox_seizing", story: gearboxOilLoss, device: "B-1",
		order:     []string{"F-1 set_feed map[feed_setpoint:54]", "F-1 set_feed map[feed_setpoint:0]", "B-1 stop_belt map[run_command:0]"},
		readBacks: [][3]string{{"set_feed", "feed_setpoint", "0"}, {"stop_belt", "run_command", "0"}},
		phases:    []string{"normal", "watch", "degrading", "shedding", "cause_gearbox", "belt_stopped"},
	}
	undecidedStopRun = plannedRun{
		degrading: "gearbox_seizing", story: undecidedDegradation, device: "B-1",
		order:     beltStopRun.order,
		readBacks: beltStopRun.readBacks,
		phases:    []string{"normal", "watch", "degrading", "shedding", "belt_empty", "cause_undecided", "belt_stopped"},
	}
)

type plannedRun struct {
	degrading string
	story     beltStory
	device    string
	order     []string
	readBacks [][3]string
	phases    []string
}

func TestTheDriveDegradationSpecSettlesEveryWindowBeforeItActs(t *testing.T) {
	t.Parallel()
	out := runCLI(t, newRootCommand(), "validate", degradationSpec, "--event-schemas", conveyorEventSchemas)
	if !strings.HasPrefix(out, "ok: drive_degradation\n") || !strings.Contains(out, "\npolicy_digest: "+conveyorPolicyDigest(t, degradationSpec)+"\n") {
		t.Fatalf("validate --event-schemas = %q, want drive_degradation and its policy digest", out)
	}
	compiled := compileConveyorSpec(t, degradationSpec)
	for _, window := range compiled.Windows {
		if window.Emit != "on_close" {
			t.Errorf("window %s emits %q, want on_close: an R2 intent is denied on provisional features", window.Name, window.Emit)
		}
	}
	if compiled.Cognition.Executor.RiskCeiling != "R2" {
		t.Errorf("risk ceiling = %q, want R2", compiled.Cognition.Executor.RiskCeiling)
	}
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestADriveDegradingAgainstItsPeersIsStoppedByPlanOnceTheBeltRunsEmpty(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	for name, planned := range map[string]plannedRun{"a dry bearing isolates the drive": isolationRun, "a gearbox losing oil stops the belt": beltStopRun,
		"a cause the evidence never names stops the belt": undecidedStopRun} {
		t.Run(name, func(t *testing.T) {
			run := startDegradationServe(t, compressedSpec(t, nil), "a", reasonAboutTheLine(compressedCoverage))
			desk := approvalDesk(run.address, conveyorApprovalToken, conveyorApprover, campaign.ApproveAll{})
			line := connectBeltLine(t, run, planned.story)
			if !line.runUntil(t, degradationTicks, run.approvingUntilSettled(t, desk, len(planned.order), planned.phases[len(planned.phases)-1])) || !slices.Equal(line.obeyed, planned.order) {
				t.Fatalf("after %d minutes the host commanded %v, want %v", line.minutes(), line.obeyed, planned.order)
			}
			for _, readBack := range planned.readBacks {
				assertTwinReadBack(t, run.db, readBack[0], readBack[1], readBack[2])
			}
			assertPhases(t, run.db, planned.phases)
			assertTheDegradingStepNames(t, run.db, planned.degrading)
			assertR2PassedOnSettledFeatures(t, run.db, 2)
			if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
				t.Fatalf("%d events quarantined, want none", quarantined)
			}
			run.stop()
			assertRunArtifactVerifies(t, experimentRun{dir: run.dir, db: run.db}, "--device-id", planned.device)
		})
	}
}

func TestFeedSurgesThatLoadEveryDriveTogetherNeverMoveTheLine(t *testing.T) {
	t.Parallel()
	run := startDegradationServe(t, compressedSpec(t, nil), "a", reasonAboutTheLine(compressedCoverage))
	line := connectBeltLine(t, run, feedSurges)
	line.runUntil(t, surgeTicks, func(time.Time) bool { return false })
	assertPhases(t, run.db, []string{"normal"})
	for _, table := range []string{"episodes", "intents", "commands", "event_quarantine"} {
		if rows := countRows(t, run.db, table); rows != 0 {
			t.Errorf("%s holds %d rows after the surges, want none", table, rows)
		}
	}
	if len(line.obeyed) != 0 {
		t.Fatalf("the host commanded %v during feed surges, want nothing", line.obeyed)
	}
}

func TestAnR2StepOnProvisionalFeaturesIsDeniedForIncompleteEvidence(t *testing.T) {
	t.Parallel()
	provisional := compressedSpec(t, map[string]string{"emit: on_close": "emit: early_and_close"})
	run := startDegradationServe(t, provisional, "a", reasonAboutTheLine(compressedCoverage))
	line := connectBeltLine(t, run, bearingDegradation)
	denied := `SELECT COUNT(*) FROM policy_evaluations p JOIN intents i ON i.intent_id = p.intent_id
		WHERE i.intent_type = 'stop_feed' AND p.result = 'denied' AND p.reason = 'source_health_incomplete'`
	if !line.runUntil(t, degradationTicks, run.holds(t, denied)) {
		t.Fatalf("after %d minutes no stop_feed intent was denied for incomplete evidence; the host obeyed %v", line.minutes(), line.obeyed)
	}
	if want := isolationRun.order[:1]; !slices.Equal(line.obeyed, want) {
		t.Fatalf("commands = %v, want only %v", line.obeyed, want)
	}
}

func compressedSpec(t *testing.T, edits map[string]string) string {
	t.Helper()
	all := maps.Clone(compressedWindows)
	maps.Copy(all, edits)
	return editedSpec(t, degradationSpec, t.TempDir(), all)
}

func bearingDegradation(minute int) beltDisturbance {
	worn := float64(max(minute-4, 0))
	return beltDisturbance{wear: min(0.05*worn, 3), heat: min(3*worn, 30)}
}

func gearboxOilLoss(minute int) beltDisturbance {
	lost := float64(max(minute-7, 0))
	return beltDisturbance{heat: min(3*lost, 30), drag: min(0.01*lost, 0.15)}
}

func undecidedDegradation(minute int) beltDisturbance {
	heated := float64(max(minute-4, 0))
	return beltDisturbance{wear: min(0.02*heated, 0.1), heat: min(3*heated, 30)}
}

func feedSurges(minute int) beltDisturbance {
	rising, falling := float64(minute-4)/6, float64(18-minute)/6
	return beltDisturbance{surge: 0.6 * max(min(rising, falling, 1), 0)}
}

type degradationServe struct {
	plantWireServe
	address string
}

func startDegradationServe(t *testing.T, specPath, plant string, reasoner workerfake.ExecuteFunc) degradationServe {
	t.Helper()
	dir := privateSocketDir(t)
	run := degradationServe{plantWireServe: plantWireServe{dir: dir, socket: filepath.Join(dir, "host.sock"), db: newMigratedDatabasePath(t)},
		address: freeLoopbackAddress(t)}
	applyPrincipalsFile(t, run.db, conveyorPrincipals)
	workerSocket := filepath.Join(dir, "worker.sock")
	serveWorker(t, workerSocket, reasoner)
	run.stop = servePlantWire(t, run.plantWireServe, run.address, specPath, conveyorKinds, conveyorSiteOf(plant), "--device-catalog", conveyorCatalogOf(plant),
		"--dispatch-lease", conveyorRouteLease, "--worker-socket", workerSocket, "--worker-name", "tamoz", "--event-schemas", conveyorEventSchemas)
	return run
}

func (r degradationServe) approvingUntilSettled(t *testing.T, desk *campaign.Desk, commands int, phase string) func(time.Time) bool {
	t.Helper()
	db := openReadOnly(t, r.db)
	settled := r.holds(t, fmt.Sprintf(`SELECT (SELECT COUNT(*) FROM commands WHERE status = 'succeeded') >= %d
		AND EXISTS (SELECT 1 FROM situations WHERE phase = '%s')`, commands, phase))
	return func(now time.Time) bool {
		if _, err := desk.Act(t.Context(), db, now); err != nil {
			t.Fatal(err)
		}
		return settled(now)
	}
}

func (r degradationServe) holds(t *testing.T, query string) func(time.Time) bool {
	t.Helper()
	db := openReadOnly(t, r.db)
	return func(time.Time) bool {
		var rows int
		if err := db.QueryRowContext(t.Context(), query).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows > 0
	}
}

func assertPhases(t *testing.T, dbPath string, want []string) {
	t.Helper()
	if got := slices.Compact(columnValues(t, dbPath, "SELECT phase FROM situation_versions ORDER BY version")); !slices.Equal(got, want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
}

func columnValues(t *testing.T, dbPath, query string) []string {
	t.Helper()
	rows, err := openReadOnly(t, dbPath).QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func assertR2PassedOnSettledFeatures(t *testing.T, dbPath string, want int) {
	t.Helper()
	db := openReadOnly(t, dbPath)
	var approved, incomplete int
	query := `SELECT (SELECT COUNT(*) FROM approvals WHERE status = 'approved'),
		(SELECT COUNT(*) FROM policy_evaluations WHERE reason = 'source_health_incomplete')`
	if err := db.QueryRowContext(t.Context(), query).Scan(&approved, &incomplete); err != nil {
		t.Fatal(err)
	}
	if approved != want || incomplete != 0 {
		t.Fatalf("%d approved R2 intents and %d denied for incomplete evidence, want %d and none", approved, incomplete, want)
	}
}

func assertTheDegradingStepNames(t *testing.T, dbPath, diagnosis string) {
	t.Helper()
	query := `SELECT d.validation_status || ' ' || json_extract(d.raw_json, '$.diagnosis_code') || ' ' || i.intent_type FROM decisions d
		JOIN situation_versions v ON v.situation_id = d.situation_id AND v.version = d.situation_version
		JOIN intents i ON i.decision_id = d.decision_id WHERE v.phase = 'degrading'`
	if got := columnValues(t, dbPath, query); !slices.Equal(got, []string{"accepted " + diagnosis + " reduce_feed"}) {
		t.Fatalf("degrading decisions = %v, want the limp step accepted with the diagnosis %s", got, diagnosis)
	}
}
