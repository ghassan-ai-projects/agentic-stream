package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

// A physical sensor never pauses: all four thermal sources keep reporting while
// the worker reasons, and every window slide publishes two Situation versions
// (provisional, then on time) although nothing material changed. Such
// versions must not cancel the decision they arrive after. zone-thermal slides
// every 30 s; this run slides every 2 s and lets the worker reason for 5 s, so
// slides always land inside the episode.
//
// Today the loop closes only because the episode runs inside the ingest batch
// and holds ingestion back while the worker reasons (G9). Once episodes run
// beside ingestion (X09) this test needs material freshness (X03) to pass. See
// docs/unfinished-work-review-2026-10-08/EXPERIMENT_DESIGN.md (G2, G9).
func TestExperimentClosedLoopUnderAContinuousFeed(t *testing.T) {
	t.Parallel()
	run := startExperiment(t, experimentOptions{specEdits: map[string]string{"slide: 30s": "slide: 2s"}, workerDelay: 5 * time.Second})
	trace := shiftedTrace(t, time.Now().Add(-time.Second))
	feedLive(t, run.liveSocket, trace)
	ctx, stopReadings := context.WithCancel(t.Context())
	defer stopReadings()
	go streamReadings(ctx, t, run.liveSocket, 300*time.Millisecond)

	waitForRow(t, run.db, "SELECT COUNT(*) FROM intents WHERE policy_status <> 'pending'", 60*time.Second)
	stopReadings()
	db := openReadOnly(t, run.db)
	var approved, commands int
	if err := db.QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM intents WHERE policy_status = 'approved'), (SELECT COUNT(*) FROM commands)").Scan(&approved, &commands); err != nil {
		t.Fatal(err)
	}
	var intentVersion, currentVersion, versions int
	_ = db.QueryRowContext(t.Context(), "SELECT (SELECT situation_version FROM intents LIMIT 1), (SELECT current_version FROM situations LIMIT 1), (SELECT COUNT(*) FROM situation_versions)").Scan(&intentVersion, &currentVersion, &versions)
	t.Logf("intent bound to v%d, current v%d, %d versions; ledger: %s", intentVersion, currentVersion, versions, ledgerSummary(t, db))
	if approved != 1 || commands != 1 {
		t.Fatalf("approved intents = %d, commands = %d under a continuous non-material feed, want 1 and 1; ledger: %s", approved, commands, ledgerSummary(t, db))
	}
}

// streamReadings sends one reading from each of the four thermal sources per
// tick, as the Streams Simulator does, until ctx ends: the temperature a
// hundredth of a degree above the last, the others steady. New facts, the same
// phase.
func streamReadings(ctx context.Context, t *testing.T, socket string, every time.Duration) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		t.Errorf("dial live socket: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for seq := 16; ; seq++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, reading := range thermalReadings(t, seq) {
				if _, err := conn.Write(append(reading, '\n')); err != nil {
					return
				}
			}
		}
	}
}

func thermalReadings(t *testing.T, seq int) [][]byte {
	sources := []struct {
		short, kind string
		data        map[string]any
	}{
		{"t", "zone.temp.observed", map[string]any{"celsius": 37.5 + float64(seq-15)/100}},
		{"a", "zone.ambient.observed", map[string]any{"celsius": 21.0}},
		{"f", "zone.fan_tach.observed", map[string]any{"rpm": 0.0, "state": "stopped"}},
		{"h", "zone.heartbeat.observed", map[string]any{}},
	}
	readings := make([][]byte, 0, len(sources))
	for _, source := range sources {
		readings = append(readings, thermalReading(t, fmt.Sprintf("live-%s-%03d", source.short, seq), source.kind, seq, source.data))
	}
	return readings
}

func thermalReading(t *testing.T, id, kind string, seq int, data map[string]any) []byte {
	now := time.Now().UTC()
	for key, value := range map[string]any{"quality": "valid", "calibration_id": "cal-1", "firmware_id": "fw-1", "boot_id": "boot-A", "schema_version": "1.0", "seq": seq} {
		data[key] = value
	}
	reading, err := json.Marshal(map[string]any{
		"id": id, "type": kind, "schema_version": "1.0", "tenant_id": "default",
		"source": "thermal-chamber-fixture", "partition_key": "zone-01", "entity": map[string]any{"type": "thermal_zone", "id": "zone-01"},
		"event_time": now.Add(-time.Second).Format(time.RFC3339Nano), "ingested_at": now.Format(time.RFC3339Nano), "classification": "internal",
		"data": data,
	})
	if err != nil {
		t.Errorf("marshal reading: %v", err)
	}
	return reading
}
