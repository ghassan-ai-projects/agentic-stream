package campaign

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func twoRunCampaign(t *testing.T) Campaign {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "campaign.json", `{"schema_version":"0.1","domain":"pump-station","seed":1,"runs":[
		{"index":1,"phase":"learn","domain":"pump-station","seed":11,"duration_s":7200,"session_plan":"run-0001.session-plan.json"},
		{"index":2,"phase":"learn","domain":"pump-station","seed":12,"duration_s":7200,"session_plan":"run-0002.session-plan.json"}]}`)
	write(t, dir, "run-0001.session-plan.json", `{"schema_version":"0.1","start_s":0}`)
	write(t, dir, "run-0002.session-plan.json", `{"schema_version":"0.1","start_s":7200}`)
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestARunsWindowStartsAtItsPlansStartAndLastsItsDuration(t *testing.T) {
	t.Parallel()
	loaded := twoRunCampaign(t)
	window, err := loaded.Window(loaded.Runs[1])
	want := Window{From: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), Until: time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC)}
	if err != nil || window != want {
		t.Fatalf("window = %+v (%v), want %+v", window, err, want)
	}
}

type decided struct {
	pump, diagnosis, at string
	version             int
}

func decide(t *testing.T, db *sql.DB, d decided) {
	t.Helper()
	entity := "site-a:" + d.pump
	situation := "sit-" + d.pump
	id := fmt.Sprintf("%s-%d", d.pump, d.version)
	statements := []string{
		fmt.Sprintf(`INSERT OR IGNORE INTO situations (situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
			current_version, phase, status, first_event_time, latest_event_time, updated_at, created_at)
			VALUES ('%s', 'default', 'dep', 'pump_condition', 'pump', '%s', 0, 'occ', 1, 'watch', 'open', '2026-01-01T00:00:10.000000000Z', '%s', '%s', '%s')`,
			situation, entity, d.at, d.at, d.at),
		fmt.Sprintf(`INSERT INTO situation_versions (situation_id, version, phase, severity, confidence, completeness, event_horizon, valid_from, snapshot_json,
			snapshot_sha256, lineage_id, created_at) VALUES ('%s', %d, 'watch', 40, 1, 'provisional', '%s', '%s', CAST('{}' AS BLOB), zeroblob(32), 'lin', '%s')`,
			situation, d.version, d.at, d.at, d.at),
		fmt.Sprintf(`INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy,
			prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at, dispatch_policy)
			VALUES ('epi-%s', 'sched-%s', 'default', '%s', %d, 'tamoz', '1', 'm', 'p', zeroblob(32), randomblob(32), CAST('{}' AS BLOB), 'concluded', '2026-10-28T00:00:00.000000000Z', 'active')`,
			id, id, situation, d.version),
		fmt.Sprintf(`INSERT INTO decisions (decision_id, episode_id, ordinal, situation_id, situation_version, raw_json, decision_sha256, validation_status, validation_json, created_at)
			VALUES ('dec-%s', 'epi-%s', 1, '%s', %d, CAST('{"diagnosis_code":"%s"}' AS BLOB), zeroblob(32), 'accepted', CAST('{}' AS BLOB), '2026-10-28T00:00:00.000000000Z')`,
			id, id, situation, d.version, d.diagnosis),
		fmt.Sprintf(`INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256, expires_at,
			policy_status, created_at, updated_at) VALUES ('int-%s', 'dec-%s', 'default', '%s', %d, 'schedule_maintenance', 'R1', CAST('{}' AS BLOB), zeroblob(32),
			'2026-10-29T00:00:00.000000000Z', 'approved', '2026-10-28T00:00:00.000000000Z', '2026-10-28T00:00:00.000000000Z')`, id, id, situation, d.version),
		fmt.Sprintf(`INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, status, created_at,
			updated_at) VALUES ('cmd-%s', 'int-%s', 'default', 'schedule_maintenance', '%s', randomblob(32), CAST('{}' AS BLOB), zeroblob(32), 'succeeded',
			'2026-10-28T00:00:00.000000000Z', '2026-10-28T00:00:00.000000000Z')`, id, id, entity),
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func readVerdict(t *testing.T, loaded Campaign, db *sql.DB, run Run) verdict {
	t.Helper()
	runDir := t.TempDir()
	write(t, runDir, "run.json", `{"run_id":"run-1"}`)
	window, err := loaded.Window(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteVerdict(t.Context(), db, runDir, "B", window); err != nil {
		t.Fatal(err)
	}
	return decodeVerdict(t, runDir)
}

func decodeVerdict(t *testing.T, runDir string) verdict {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, verdictFile))
	if err != nil {
		t.Fatal(err)
	}
	var v verdict
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAVerdictPlacesDetectionsAndActionsInTheRunsSimulatedWindow(t *testing.T) {
	t.Parallel()
	loaded := twoRunCampaign(t)
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	at := func(clock string) string {
		parsed, err := time.Parse(time.RFC3339, "2026-01-01T"+clock+"Z")
		if err != nil {
			t.Fatal(err)
		}
		return kernel.FormatTime(parsed)
	}
	decide(t, db, decided{pump: "pump-3", diagnosis: "bearing_wear", at: at("00:40:00"), version: 1})
	decide(t, db, decided{pump: "pump-3", diagnosis: "misalignment", at: at("02:10:00"), version: 2})
	decide(t, db, decided{pump: "pump-5", diagnosis: "sensor_drift", at: at("02:20:00"), version: 1})
	decide(t, db, decided{pump: "pump-5", diagnosis: "bearing_wear", at: at("03:00:00"), version: 2})

	first := readVerdict(t, loaded, db, loaded.Runs[0])
	if len(first.Detections) != 1 || first.Detections[0] != (detection{EntityID: "site-a/pump-3", DetectedAt: at("00:40:00"), Label: "bearing_wear"}) {
		t.Fatalf("run 1 detections = %+v, want pump-3 at 00:40 as bearing_wear", first.Detections)
	}
	if len(first.Actions) != 1 || first.Actions[0].IssuedAt != at("00:40:00") || first.Actions[0].EntityID != "site-a/pump-3" {
		t.Fatalf("run 1 actions = %+v, want the one command issued at 00:40", first.Actions)
	}
	second := readVerdict(t, loaded, db, loaded.Runs[1])
	wantDetections := []detection{{EntityID: "site-a/pump-3", DetectedAt: at("02:10:00"), Label: "misalignment"}, {EntityID: "site-a/pump-5", DetectedAt: at("02:20:00"), Label: "sensor_drift"},
		{EntityID: "site-a/pump-5", DetectedAt: at("03:00:00"), Label: "bearing_wear"}}
	if !slices.Equal(second.Detections, wantDetections) {
		t.Fatalf("run 2 detections = %+v, want %+v: the first decision of each diagnosis on each pump in the run", second.Detections, wantDetections)
	}
	if len(second.Actions) != 3 || second.Actions[0].IssuedAt != at("02:10:00") || second.Actions[1].IssuedAt != at("02:20:00") || second.Actions[2].IssuedAt != at("03:00:00") {
		t.Fatalf("run 2 actions = %+v, want the three commands decided in the run, issued in event time", second.Actions)
	}
}

func TestAVerdictReportsTheSubjectsItIsGiven(t *testing.T) {
	t.Parallel()
	loaded := twoRunCampaign(t)
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	decide(t, db, decided{pump: "pump-3", diagnosis: "drive_tripped", at: "2026-01-01T00:40:00.000000000Z", version: 1})
	runDir := t.TempDir()
	write(t, runDir, "run.json", `{"run_id":"run-1"}`)
	window, err := loaded.Window(loaded.Runs[0])
	if err != nil {
		t.Fatal(err)
	}
	subjects := Subjects{Detected: func(_, diagnosis string) (string, string) { return "D-3", "label-" + diagnosis },
		Acted: func(route, _ string) (string, string) { return "device-of-" + route, "raw-" + route }}
	if err := WriteVerdictOf(t.Context(), db, runDir, window, Verdict{Version: "m2", Arm: "limp", Subjects: subjects}); err != nil {
		t.Fatal(err)
	}
	v := decodeVerdict(t, runDir)
	if v.Consumer != (consumer{Name: "agentic-stream", Version: "m2", Arm: "limp"}) || len(v.Detections) != 1 || len(v.Actions) != 1 {
		t.Fatalf("verdict = %+v, want version m2, arm limp, one detection and one action", v)
	}
	if v.Detections[0].EntityID != "D-3" || v.Detections[0].Label != "label-drive_tripped" || v.Actions[0].EntityID != "device-of-schedule_maintenance" ||
		v.Actions[0].Effector != "raw-schedule_maintenance" {
		t.Fatalf("verdict rows = %+v, %+v; want the detection on D-3 as label-drive_tripped and the action on its route's device and effector", v.Detections, v.Actions)
	}
}

func TestAVerdictFoldsASymptomIntoTheLaterCauseOfItsInstanceAndLeavesOutWhatNamesNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	decide(t, db, decided{pump: "pump-3", diagnosis: "unknown", at: "2026-01-01T00:30:00.000000000Z", version: 1})
	decide(t, db, decided{pump: "pump-3", diagnosis: "degrading", at: "2026-01-01T00:40:00.000000000Z", version: 2})
	decide(t, db, decided{pump: "pump-3", diagnosis: "bearing_wear", at: "2026-01-01T01:30:00.000000000Z", version: 3})
	decide(t, db, decided{pump: "pump-5", diagnosis: "degrading", at: "2026-01-01T00:50:00.000000000Z", version: 1})
	got := foldedDetections(t, db, map[string]string{"degrading": "", "bearing_wear": "pump__bearing"}, map[string]string{"bearing_wear": "degrading"})
	want := []detection{{EntityID: "site-a:pump-3", DetectedAt: "2026-01-01T00:40:00.000000000Z", Label: "pump__bearing"},
		{EntityID: "site-a:pump-5", DetectedAt: "2026-01-01T00:50:00.000000000Z"}}
	if !slices.Equal(got, want) {
		t.Fatalf("detections = %+v, want %+v: the symptom dates pump-3's bearing, stands alone on pump-5, and unknown is left out", got, want)
	}
}

func TestAVerdictFoldsASymptomOnlyIntoTheCausesThatRefineIt(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	decide(t, db, decided{pump: "pump-3", diagnosis: "drive_degrading", at: "2026-01-01T00:40:00.000000000Z", version: 1})
	decide(t, db, decided{pump: "pump-3", diagnosis: "sensor_stuck", at: "2026-01-01T01:00:00.000000000Z", version: 2})
	decide(t, db, decided{pump: "pump-3", diagnosis: "bearing", at: "2026-01-01T01:30:00.000000000Z", version: 3})
	labels := map[string]string{"drive_degrading": "", "bearing": "pump__lube_loss", "sensor_stuck": "pump__vibration__frozen"}
	got := foldedDetections(t, db, labels, map[string]string{"bearing": "drive_degrading", "gearbox_seizing": "drive_degrading"})
	want := []detection{{EntityID: "site-a:pump-3", DetectedAt: "2026-01-01T00:40:00.000000000Z", Label: "pump__lube_loss"},
		{EntityID: "site-a:pump-3", DetectedAt: "2026-01-01T01:00:00.000000000Z", Label: "pump__vibration__frozen"}}
	if !slices.Equal(got, want) {
		t.Fatalf("detections = %+v, want %+v: the bearing refines drive_degrading and takes its time, sensor_stuck refines nothing and keeps its own", got, want)
	}
}

func foldedDetections(t *testing.T, db *sql.DB, labels, refines map[string]string) []detection {
	t.Helper()
	loaded := twoRunCampaign(t)
	runDir := t.TempDir()
	write(t, runDir, "run.json", `{"run_id":"run-1"}`)
	window, err := loaded.Window(loaded.Runs[0])
	if err != nil {
		t.Fatal(err)
	}
	subjects := Subjects{
		Detected: func(entityID, diagnosis string) (string, string) {
			if label, known := labels[diagnosis]; known {
				return entityID, label
			}
			return "", ""
		},
		Acted:   func(route, _ string) (string, string) { return route, route },
		Refines: refines,
	}
	if err := WriteVerdictOf(t.Context(), db, runDir, window, Verdict{Version: "v", Arm: "a", Subjects: subjects}); err != nil {
		t.Fatal(err)
	}
	return decodeVerdict(t, runDir).Detections
}
