package replay_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"

	_ "modernc.org/sqlite"
)

// The thermal-chamber telemetry vertical (Real-World Sensor HIL-0, Phase 01 /
// Gate G2). These tests prove that a real over-temp opens a Situation, that a
// noisy single reading and a quality-flagged disconnected sensor do NOT, that
// quality-gated samples are still retained as evidence, and that the ambient
// discount rejects a merely ambient-tracking rise before opening. No model, no
// effector — this is the deterministic engine only.
//
// docs/plans/real-world-sensor-hil/01-telemetry-vertical.md

const thermalSpec = "../../docs/design/examples/zone-thermal.situation.yaml"

func thermalTrace(name string) string {
	return filepath.Join("../../examples/thermal-chamber/testdata", name)
}

func TestThermalChamberOpensOnlyOnRealOverTemp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		trace      string
		wantOpens  bool
		wantEvents int
	}{
		// A sustained rise with a flat ambient is a real over-temp.
		{"trace-opening.jsonl", true, 64},
		// A stable zone with one lone high reading must not open: the 5-minute
		// mean never crosses the band and the slope is not sustained.
		{"trace-quiet.jsonl", false, 64},
		// A disconnected sensor reports quality flags with no celsius value. The
		// quality gate skips them, so no Situation opens — yet every
		// event is still ingested and retained as evidence.
		{"trace-invalid-quality.jsonl", false, 64},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.trace, func(t *testing.T) {
			t.Parallel()
			dbPath := filepath.Join(t.TempDir(), "replay.db")
			result, err := replay.Run(context.Background(), dbPath, thermalSpec, thermalTrace(tc.trace), "default")
			if err != nil {
				t.Fatalf("replay %s: %v", tc.trace, err)
			}
			if result.EventsProcessed != tc.wantEvents {
				t.Fatalf("events_processed = %d, want %d (quality-flagged samples must still be ingested as evidence)", result.EventsProcessed, tc.wantEvents)
			}
			opened := result.VersionCount > 0
			if opened != tc.wantOpens {
				t.Fatalf("opened = %v (versions=%d), want %v", opened, result.VersionCount, tc.wantOpens)
			}
		})
	}
}

// The ambient cross-check is an occurrence gate: a genuine over-band rise with
// a flat ambient opens a Situation, but a rise that merely tracks a rising room
// does not open one.
func TestThermalChamberAmbientDiscountHoldsInWatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		trace     string
		wantPhase string
	}{
		{"trace-opening.jsonl", "cooling"},
		{"trace-ambient-tracking.jsonl", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.trace, func(t *testing.T) {
			t.Parallel()
			dbPath := filepath.Join(t.TempDir(), "replay.db")
			if _, err := replay.Run(context.Background(), dbPath, thermalSpec, thermalTrace(tc.trace), "default"); err != nil {
				t.Fatalf("replay %s: %v", tc.trace, err)
			}
			if tc.wantPhase == "" {
				var count int
				if err := queryThermalDB(t, dbPath, "SELECT COUNT(*) FROM situations WHERE situation_type = 'zone_over_temp'", &count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("ambient-tracking trace created %d situations, want none", count)
				}
				return
			}
			phase := finalThermalPhase(t, dbPath)
			if phase != tc.wantPhase {
				t.Fatalf("final phase = %q, want %q", phase, tc.wantPhase)
			}
		})
	}
}

func TestThermalChamberRebootWrapAndBacklogStayBootScoped(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "reboot-backlog.db")
	result, err := replay.Run(context.Background(), dbPath, thermalSpec, thermalTrace("trace-reboot-backlog.jsonl"), "default")
	if err != nil {
		t.Fatalf("replay reboot/backlog trace: %v", err)
	}
	if result.EventsProcessed != 12 {
		t.Fatalf("events_processed = %d, want 12", result.EventsProcessed)
	}
	if result.VersionCount != 0 {
		t.Fatalf("cross-boot stale high reading opened %d situation versions", result.VersionCount)
	}
}

func queryThermalDB(t *testing.T, dbPath, query string, dest *int) error {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", dbPath))
	if err != nil {
		return fmt.Errorf("open replay db: %w", err)
	}
	defer func() { _ = db.Close() }()
	return db.QueryRowContext(context.Background(), query).Scan(dest)
}

// Replay of the thermal spec is byte-stable across runs — the deterministic
// replay contract, on a physical-evidence domain.
func TestThermalChamberReplayIsDeterministic(t *testing.T) {
	t.Parallel()
	results, err := replay.RunNTimes(context.Background(), thermalSpec, thermalTrace("trace-opening.jsonl"), "default", 3)
	if err != nil {
		t.Fatalf("run n times: %v", err)
	}
	for i := 1; i < len(results); i++ {
		if results[i].VersionsHash != results[0].VersionsHash || results[i].VersionCount != results[0].VersionCount {
			t.Fatalf("run %d = (versions=%d hash=%s), run 0 = (versions=%d hash=%s)",
				i, results[i].VersionCount, results[i].VersionsHash, results[0].VersionCount, results[0].VersionsHash)
		}
	}
}

func TestThermalReplayQuarantinesMalformedInputAfterSchemaSetup(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "malformed.jsonl")
	valid := `{"id":"evt-valid","type":"zone.temp.observed","schema_version":"1.0","tenant_id":"default","source":"thermal-chamber-test","partition_key":"zone-01","entity":{"type":"thermal_zone","id":"zone-01"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"celsius":25,"quality":"valid","calibration_id":"cal-1","firmware_id":"fw-1","schema_version":"1.0","raw_value":25,"boot_id":"boot-A","seq":1,"device_mono_us":1000000}}` + "\n"
	if err := os.WriteFile(path, []byte(valid+"not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "replay.db")
	result, err := replay.Run(context.Background(), dbPath, thermalSpec, path, "default")
	if err != nil {
		t.Fatalf("replay malformed trace: %v", err)
	}
	if result.EventsProcessed != 1 {
		t.Fatalf("events_processed = %d, want 1 valid event", result.EventsProcessed)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var events, quarantined int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM event_log").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM event_quarantine WHERE reason_code = 'malformed_json'").Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if events != 1 || quarantined != 1 {
		t.Fatalf("event retention = %d, quarantine = %d; want 1 and 1", events, quarantined)
	}
}

func finalThermalPhase(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", dbPath))
	if err != nil {
		t.Fatalf("open replay db: %v", err)
	}
	defer func() { _ = db.Close() }()
	var phase string
	if err := db.QueryRowContext(context.Background(), `SELECT phase FROM situations WHERE situation_type = 'zone_over_temp'`).Scan(&phase); err != nil {
		t.Fatalf("query final phase: %v", err)
	}
	return phase
}
