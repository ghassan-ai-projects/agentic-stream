package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func pumpSite() map[string]any {
	return map[string]any{
		"version": 1,
		"source":  "plantwire-test",
		"devices": map[string]any{"P1": map[string]any{"kind": "pump", "entity": map[string]any{"type": "station", "id": "station-1"}}},
		"readings": []any{map[string]any{"kind": "pump", "metric": "flow", "event_type": "pump.flow.observed", "schema_version": "1.0",
			"field": "m3_per_hour"}},
		"heartbeats": []any{map[string]any{"kind": "pump", "event_type": "pump.heartbeat.observed", "schema_version": "1.0",
			"every_ms": 10000, "stale_after_ms": 6000}},
		"quality": map[string]any{"good": "valid", "warming_up": "warming", "suspect": "invalid", "out_of_range": "invalid",
			"stale": "invalid", "disconnected": "disconnected", "manual_override": "invalid", "out_of_service": "disconnected"},
	}
}

func siteFile(t *testing.T, site map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(site)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.site.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
