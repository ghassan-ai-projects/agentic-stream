package app_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	simulatorConfig = `{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}`
	simulatorEnd    = `{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}`
)

func simulatorEvent(id, entityType, channel, eventTime, arrival string, value float64) string {
	event := map[string]any{"id": id, "entity_type": entityType, "entity_id": "site-a." + entityType + "-1", "type": channel, "event_time": eventTime, "arrival_time": arrival, "value": value}
	line, err := json.Marshal(map[string]any{"record_type": "event", "event": event})
	if err != nil {
		panic(err)
	}
	return string(line)
}

type storedEvent struct {
	Type   string
	Entity string
	Data   map[string]any
}

func storedEvents(t *testing.T, db *storage.DB) map[string]storedEvent {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT event_id, event_type, entity_type, payload_json FROM event_log ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	events := map[string]storedEvent{}
	for rows.Next() {
		var id string
		var event storedEvent
		var payload []byte
		if err := rows.Scan(&id, &event.Type, &event.Entity, &payload); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &event.Data); err != nil {
			t.Fatalf("decode payload for %q: %v", id, err)
		}
		events[id] = event
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestSimulatorReplayConvertsControlsAndEvents(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	path := writeTrace(t, joinLines(
		simulatorConfig,
		simulatorEvent("evt-pond-000001", "pond", "pond.dissolved_oxygen", "2026-07-29T09:00:00Z", "2026-07-29T09:00:01.5Z", 6.38),
		simulatorEvent("evt-pump-000001", "pump", "vibration", "2026-07-29T09:00:02Z", "2026-07-29T09:00:03Z", 5.2),
		simulatorEvent("evt-pump-pressure-000001", "pump", "discharge_pressure", "2026-07-29T09:00:04Z", "2026-07-29T09:00:05Z", 245.5),
		simulatorEvent("evt-bay-humidity-000001", "bay", "bay.humidity", "2026-07-29T09:00:06Z", "2026-07-29T09:00:07Z", 78.4),
		simulatorEnd,
	))
	replay := newSimulator(t, db, domain.SimulatorOptions{TenantID: "default"}, path, "test-sim")

	if count, err := replay.Run(t.Context()); err != nil || count != 4 {
		t.Fatalf("count = %d, err = %v, want the 4 events and none of the framing records", count, err)
	}

	want := map[string]storedEvent{
		"evt-pond-000001":          {Type: "pond.dissolved_oxygen.observed", Entity: "pond", Data: map[string]any{"mg_l": 6.38}},
		"evt-pump-000001":          {Type: "pump.vibration.observed", Entity: "pump", Data: map[string]any{"rms_mm_s": 5.2}},
		"evt-pump-pressure-000001": {Type: "pump.discharge_pressure.observed", Entity: "pump", Data: map[string]any{"kpa": 245.5}},
		"evt-bay-humidity-000001":  {Type: "bay.humidity.observed", Entity: "bay", Data: map[string]any{"percent": 78.4}},
	}
	if got := storedEvents(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("stored events = %+v, want %+v", got, want)
	}
	if count, err := replay.Run(t.Context()); err != nil || count != 0 {
		t.Fatalf("repeat count = %d, err = %v, want a resumed replay to append nothing", count, err)
	}
}

func TestSimulatorReplayRefusesATraceThatBreaksTheGrammar(t *testing.T) {
	t.Parallel()
	event := simulatorEvent("evt-1", "pump", "vibration", "2026-07-29T09:00:00Z", "2026-07-29T09:00:01Z", 5.2)
	flattened := `{"record_type":"event","event.id":"evt-1","event.entity_type":"pump","event.entity_id":"pump-1","event.type":"vibration","event.event_time":"2026-07-29T09:00:00Z","event.arrival_time":"2026-07-29T09:00:01Z","event.value":5.2}`
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "flattened event fields", content: joinLines(simulatorConfig, flattened, simulatorEnd), want: `unknown event field "event.`},
		{name: "unknown record type", content: joinLines(simulatorConfig, `{"record_type":"unknown"}`, simulatorEnd), want: `unknown simulator record_type "unknown"`},
		{name: "no runtime_config first", content: joinLines(event, simulatorEnd), want: "runtime_config must be first"},
		{name: "no trace_end last", content: joinLines(simulatorConfig, event), want: "runtime_config first and trace_end last"},
		{name: "empty trace", content: "", want: "runtime_config first and trace_end last"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			replay := newSimulator(t, db, domain.SimulatorOptions{}, writeTrace(t, tt.content), "test-sim")

			count, err := replay.Run(t.Context())

			if err == nil || !strings.Contains(err.Error(), tt.want) || count != 0 {
				t.Fatalf("count = %d, err = %v, want a refusal containing %q", count, err, tt.want)
			}
			if events, checkpoints := countRows(t, db, "event_log"), countRows(t, db, "connector_checkpoints"); events != 0 || checkpoints != 0 {
				t.Fatalf("a refused trace left %d events and %d checkpoints", events, checkpoints)
			}
		})
	}
}

func TestSimulatorReplayNamesAMissingTrace(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	replay := newSimulator(t, db, domain.SimulatorOptions{}, filepath.Join(t.TempDir(), "absent.jsonl"), "")

	_, err := replay.Run(t.Context())

	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "open simulator trace") {
		t.Fatalf("err = %v, want fs.ErrNotExist wrapped by open simulator trace", err)
	}
}
