package domain

import (
	"strings"
	"testing"
)

func simulatorEventRecord(edit func(event map[string]any)) map[string]any {
	event := map[string]any{
		"id": "evt-1", "entity_type": "pump", "entity_id": "pump-1",
		"type": "temperature", "event_time": "2026-07-29T09:00:00Z",
		"arrival_time": "2026-07-29T09:00:01Z", "value": 26.5,
	}
	if edit != nil {
		edit(event)
	}
	return map[string]any{"record_type": "event", "event": event}
}

func TestConvertEventMapsTheChannelValueToItsConfiguredField(t *testing.T) {
	t.Parallel()
	options := SimulatorOptions{TenantID: "acme", EntityType: "pump", Source: "sim", EventTypePrefix: "pump."}.Normalized()
	tests := []struct {
		name  string
		edit  func(event map[string]any)
		wants map[string]any
	}{
		{name: "mapped channel", wants: map[string]any{"celsius": 26.5}},
		{name: "mapped channel with a unit", edit: func(e map[string]any) { e["unit"] = "degC" }, wants: map[string]any{"celsius": 26.5, "unit": "degC"}},
		{name: "mode lands in value and mode", edit: func(e map[string]any) { e["type"], e["value"] = "mode", "auto" }, wants: map[string]any{"value": "auto", "mode": "auto"}},
		{name: "heartbeat carries no field", edit: func(e map[string]any) { e["type"] = "heartbeat" }, wants: map[string]any{}},
		{name: "unknown channel falls back to value", edit: func(e map[string]any) { e["type"], e["value"] = "mystery_channel", 7.0 }, wants: map[string]any{"value": 7.0}},
		{name: "event without a value", edit: func(e map[string]any) { delete(e, "value") }, wants: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envelope, err := options.ConvertEvent(simulatorEventRecord(tt.edit))
			if err != nil {
				t.Fatal(err)
			}
			if len(envelope.Data) != len(tt.wants) {
				t.Fatalf("data = %v, want %v", envelope.Data, tt.wants)
			}
			for key, want := range tt.wants {
				if envelope.Data[key] != want {
					t.Fatalf("data = %v, want %v", envelope.Data, tt.wants)
				}
			}
		})
	}
}

func TestConvertEventNamesTheEventTypeFromThePrefixAndChannel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options SimulatorOptions
		channel string
		want    string
	}{
		{name: "entity type as prefix", options: SimulatorOptions{}, channel: "vibration", want: "pump.vibration.observed"},
		{name: "configured prefix", options: SimulatorOptions{EventTypePrefix: "plant."}, channel: "vibration", want: "plant.vibration.observed"},
		{name: "channel already carries the prefix", options: SimulatorOptions{}, channel: "pump.vibration", want: "pump.vibration.observed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envelope, err := tt.options.Normalized().ConvertEvent(simulatorEventRecord(func(e map[string]any) { e["type"] = tt.channel }))
			if err != nil || envelope.Type != tt.want {
				t.Fatalf("type = %q, err = %v, want %q", envelope.Type, err, tt.want)
			}
		})
	}
}

func TestConvertEventRefusesAnEventThatBreaksTheRecordGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		record map[string]any
		want   string
	}{
		{name: "unknown record field", record: map[string]any{"record_type": "event", "event": simulatorEventRecord(nil)["event"], "extra": 1}, want: `unknown event field "extra"`},
		{name: "unknown event field", record: simulatorEventRecord(func(e map[string]any) { e["extra"] = 1 }), want: `unknown event field "extra"`},
		{name: "event object missing", record: map[string]any{"record_type": "event"}, want: "event is required"},
		{name: "missing id", record: simulatorEventRecord(func(e map[string]any) { delete(e, "id") }), want: "id is required"},
		{name: "empty entity id", record: simulatorEventRecord(func(e map[string]any) { e["entity_id"] = "" }), want: "entity_id is required"},
		{name: "missing entity type", record: simulatorEventRecord(func(e map[string]any) { delete(e, "entity_type") }), want: "entity_type is required"},
		{name: "missing channel", record: simulatorEventRecord(func(e map[string]any) { delete(e, "type") }), want: "type is required"},
		{name: "entity type differs from the configured one", record: simulatorEventRecord(func(e map[string]any) { e["entity_type"] = "pond" }), want: `entity_type "pond" does not match configured type "pump"`},
		{name: "missing event time", record: simulatorEventRecord(func(e map[string]any) { delete(e, "event_time") }), want: "event_time is required"},
		{name: "unparseable event time", record: simulatorEventRecord(func(e map[string]any) { e["event_time"] = "noon" }), want: "parse event_time"},
		{name: "missing arrival time", record: simulatorEventRecord(func(e map[string]any) { delete(e, "arrival_time") }), want: "arrival_time is required"},
		{name: "arrival before the event", record: simulatorEventRecord(func(e map[string]any) { e["arrival_time"] = "2026-07-29T08:59:59Z" }), want: "arrival precedes event time"},
	}
	options := SimulatorOptions{EntityType: "pump"}.Normalized()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := options.ConvertEvent(tt.record); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
