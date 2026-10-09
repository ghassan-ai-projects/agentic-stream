package replay_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

const activeThermalSpec = "../../examples/real-world-sensor/zone-thermal-sim.situation.yaml"

func TestDeterministicReplayOfAnEffectProducingTraceWritesNoIntentOrCommand(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "replay.db")
	result, err := replay.Run(seededContext(t), replay.Request{DBPath: dbPath, SpecPath: activeThermalSpec, TracePath: thermalTrace("trace-opening.jsonl"), TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if result.EffectsAllowed || result.WorkerInvoked || result.CapabilityCalls != 0 {
		t.Fatalf("deterministic replay crossed a boundary: %+v", result)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var phase string
	if err := db.QueryRowContext(t.Context(), "SELECT phase FROM situations WHERE situation_type = 'zone_over_temp'").Scan(&phase); err != nil || phase != "cooling" {
		t.Fatalf("replayed phase = %q err=%v, want cooling: the phase the live closed loop dispatches a command from", phase, err)
	}
	for _, table := range []string{"episodes", "decisions", "intents", "commands", "outbox", "outcomes"} {
		var rows int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("deterministic replay wrote %d rows into %s (err %v)", rows, table, err)
		}
	}
}
