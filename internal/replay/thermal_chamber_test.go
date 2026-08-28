package replay_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"

	_ "modernc.org/sqlite"
)

// The thermal-chamber telemetry vertical (Real-World Sensor HIL-0, Phase 01 /
// Gate G2). These tests prove that a real over-temp opens a Situation, that a
// noisy single reading and a quality-flagged disconnected sensor do NOT, that
// quality-gated samples are still retained as evidence, and that the ambient
// discount holds a merely ambient-tracking rise in `watch`. No model, no
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
		// aggregates skip valueless samples, so no Situation opens — yet every
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

// The ambient cross-check is a phase-level discount, not an occurrence gate: a
// genuine over-band rise still opens a Situation, but a rise that merely tracks
// a rising room is held in `watch` and never escalated to `rising`.
func TestThermalChamberAmbientDiscountHoldsInWatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		trace     string
		wantPhase string
	}{
		{"trace-opening.jsonl", "rising"},
		{"trace-ambient-tracking.jsonl", "watch"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.trace, func(t *testing.T) {
			t.Parallel()
			dbPath := filepath.Join(t.TempDir(), "replay.db")
			if _, err := replay.Run(context.Background(), dbPath, thermalSpec, thermalTrace(tc.trace), "default"); err != nil {
				t.Fatalf("replay %s: %v", tc.trace, err)
			}
			phase := finalThermalPhase(t, dbPath)
			if phase != tc.wantPhase {
				t.Fatalf("final phase = %q, want %q", phase, tc.wantPhase)
			}
		})
	}
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

func finalThermalPhase(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", dbPath))
	if err != nil {
		t.Fatalf("open replay db: %v", err)
	}
	defer func() { _ = db.Close() }()
	var phase string
	if err := db.QueryRow(`SELECT phase FROM situations WHERE situation_type = 'zone_over_temp'`).Scan(&phase); err != nil {
		t.Fatalf("query final phase: %v", err)
	}
	return phase
}
