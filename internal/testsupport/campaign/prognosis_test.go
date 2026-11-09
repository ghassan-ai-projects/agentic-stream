package campaign

import (
	"database/sql"
	"fmt"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAVerdictCarriesTheFailureTimeTheSituationPredictedAtEachDetection(t *testing.T) {
	t.Parallel()
	loaded := twoRunCampaign(t)
	db := storagetest.OpenTempWithoutForeignKeys(t).DB
	decide(t, db, decided{pump: "pump-3", diagnosis: "bearing_wear", at: "2026-01-01T00:40:00.000000000Z", version: 1})
	predictAt(t, db, "pump-3", 1, "", 3600)
	for version, p := range []struct {
		at      string
		seconds float64
	}{{"00:45", 3000}, {"00:50", 2700}, {"01:00", 0}, {"01:05", 1800}} {
		predictAt(t, db, "pump-3", version+2, "2026-01-01T"+p.at+":00.000000000Z", p.seconds)
	}
	decide(t, db, decided{pump: "pump-5", diagnosis: "sensor_drift", at: "2026-01-01T00:55:00.000000000Z", version: 1})
	runDir := t.TempDir()
	write(t, runDir, "run.json", `{"run_id":"run-1"}`)
	window, err := loaded.Window(loaded.Runs[0])
	if err != nil {
		t.Fatal(err)
	}
	subjects := SameNames
	subjects.Prognosis = map[string]string{"bearing_wear": "to_failure_s"}
	if err := WriteVerdictOf(t.Context(), db, runDir, window, Verdict{Version: "v", Arm: "a", Subjects: subjects}); err != nil {
		t.Fatal(err)
	}
	want := []detection{
		{EntityID: "site-a/pump-3", DetectedAt: "2026-01-01T00:40:00.000000000Z", Label: "bearing_wear", PredictedFailureAt: "2026-01-01T01:40:00.000000000Z"},
		{EntityID: "site-a/pump-3", DetectedAt: "2026-01-01T00:50:00.000000000Z", Label: "bearing_wear", PredictedFailureAt: "2026-01-01T01:35:00.000000000Z"},
		{EntityID: "site-a/pump-5", DetectedAt: "2026-01-01T00:55:00.000000000Z", Label: "sensor_drift"},
		{EntityID: "site-a/pump-3", DetectedAt: "2026-01-01T01:05:00.000000000Z", Label: "bearing_wear", PredictedFailureAt: "2026-01-01T01:35:00.000000000Z"},
	}
	if got := decodeVerdict(t, runDir).Detections; !slices.Equal(got, want) {
		t.Fatalf("detections = %+v\nwant %+v: one prediction per 10 minutes while valid, none for a label without a prognosis", got, want)
	}
}

func predictAt(t *testing.T, db *sql.DB, pump string, version int, at string, seconds float64) {
	t.Helper()
	snapshot := fmt.Sprintf(`{"facts":{"facts.to_failure_s":%g}}`, seconds)
	statement := `UPDATE situation_versions SET snapshot_json = CAST(?1 AS BLOB) WHERE situation_id = ?2 AND version = ?3`
	args := []any{snapshot, "sit-" + pump, version}
	if at != "" {
		statement = `INSERT INTO situation_versions (situation_id, version, phase, severity, confidence, completeness, event_horizon, valid_from, snapshot_json,
			snapshot_sha256, lineage_id, created_at) VALUES (?2, ?3, 'watch', 40, 1, 'final_by_policy', ?4, ?4, CAST(?1 AS BLOB), zeroblob(32), 'lin', ?4)`
		args = append(args, at)
	}
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("record the prediction of %s version %d: %v", pump, version, err)
	}
}
