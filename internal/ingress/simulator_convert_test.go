package ingress

import (
	"testing"
)

// Domain-data extraction: the channel→field mapping is data-driven. These
// cases pin the machinery edges the trace fixture does not exercise — the
// mode double-write, the heartbeat no-data sentinel, the unknown-channel
// value fallback, and the unit passthrough. In-package so convertEvent is
// reachable without a test-only production API.
func TestSimulatorChannelFieldMappingEdges(t *testing.T) {
	replay := NewSimulatorJSONLReplay(nil, nil, SimulatorOptions{
		TenantID: "acme", EntityType: "pump", Source: "sim", EventTypePrefix: "pump.",
	}, "", "test-sim")

	convert := func(channel string, value any, unit string) map[string]any {
		event := map[string]any{
			"id": "evt-1", "entity_type": "pump", "entity_id": "pump-1",
			"type": channel, "event_time": "2026-07-29T09:00:00Z",
			"arrival_time": "2026-07-29T09:00:01Z", "value": value,
		}
		if unit != "" {
			event["unit"] = unit
		}
		envelope, err := replay.convertEvent(map[string]any{"event": event})
		if err != nil {
			t.Fatalf("convert %s: %v", channel, err)
		}
		return envelope.Data
	}

	// mode: the value lands in data["value"] AND data["mode"].
	mode := convert("mode", "auto", "")
	if mode["value"] != "auto" || mode["mode"] != "auto" {
		t.Fatalf("mode = %+v, want both value and mode", mode)
	}

	// heartbeat: no data field at all.
	heartbeat := convert("heartbeat", 1.0, "")
	if len(heartbeat) != 0 {
		t.Fatalf("heartbeat = %+v, want empty data", heartbeat)
	}

	// unknown channel: falls back to data["value"].
	unknown := convert("mystery_channel", 7.0, "")
	if unknown["value"] != 7.0 {
		t.Fatalf("unknown channel = %+v, want value fallback", unknown)
	}

	// unit passthrough applies to every channel alongside the mapped field.
	withUnit := convert("temperature", 26.5, "degC")
	if withUnit["celsius"] != 26.5 {
		t.Fatalf("temperature = %+v, want celsius", withUnit)
	}
	if withUnit["unit"] != "degC" {
		t.Fatalf("temperature = %+v, want unit passthrough", withUnit)
	}
}
