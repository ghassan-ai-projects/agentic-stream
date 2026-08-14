package ingress_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSimulatorJSONLReplayConvertsControlsAndEvents(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "sim.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	content := `{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}
{"record_type":"event","event":{"id":"evt-pond-000001","entity_type":"pond","entity_id":"site-a.pond-1","type":"pond.dissolved_oxygen","event_time":"2026-07-29T09:00:00Z","arrival_time":"2026-07-29T09:00:01.5Z","value":6.38}}
{"record_type":"event","event":{"id":"evt-pump-000001","entity_type":"pump","entity_id":"site-a.pump-1","type":"vibration","event_time":"2026-07-29T09:00:02Z","arrival_time":"2026-07-29T09:00:03Z","value":5.2}}
{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	replay := ingress.NewSimulatorJSONLReplay(db, eventlog.NewEventLog(db), ingress.SimulatorOptions{TenantID: "default"}, path, "test-sim")
	count, err := replay.Run(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	rows, err := db.QueryContext(context.Background(), "SELECT event_id, event_type, entity_type, payload_json FROM event_log ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	expected := map[string]struct {
		typ    string
		entity string
		data   map[string]any
	}{
		"evt-pond-000001": {typ: "pond.dissolved_oxygen.observed", entity: "pond", data: map[string]any{"mg_l": 6.38}},
		"evt-pump-000001": {typ: "pump.vibration.observed", entity: "pump", data: map[string]any{"rms_mm_s": 5.2}},
	}
	for rows.Next() {
		var eventID, typ, entity string
		var payload []byte
		if err := rows.Scan(&eventID, &typ, &entity, &payload); err != nil {
			t.Fatal(err)
		}
		want, ok := expected[eventID]
		if !ok {
			t.Fatalf("unexpected event id %q", eventID)
		}
		var data map[string]any
		if err := json.Unmarshal(payload, &data); err != nil {
			t.Fatalf("decode payload for %q: %v", eventID, err)
		}
		if typ != want.typ || entity != want.entity {
			t.Fatalf("event %q type/entity = %q/%q, want %q/%q", eventID, typ, entity, want.typ, want.entity)
		}
		if !reflect.DeepEqual(data, want.data) {
			t.Fatalf("event %q data = %v, want %v", eventID, data, want.data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	count, err = replay.Run(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("repeat count=%d err=%v", count, err)
	}
}

func TestSimulatorJSONLReplayRejectsFlattenedAndOutOfOrderRecords(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "sim.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	content := `{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}
{"record_type":"event","event.id":"evt-000001","event.entity_type":"pump","event.entity_id":"pump-1","event.type":"vibration","event.event_time":"2026-07-29T09:00:00Z","event.arrival_time":"2026-07-29T09:00:01Z","event.value":5.2,"event.unit":"mm/s"}
{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	replay := ingress.NewSimulatorJSONLReplay(db, eventlog.NewEventLog(db), ingress.SimulatorOptions{}, path, "test-sim")
	if _, err := replay.Run(context.Background()); err == nil {
		t.Fatal("expected flattened event rejection")
	}
}

func TestSimulatorJSONLReplayRejectsUnknownRecord(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "sim.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte(`{"record_type":"unknown"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	replay := ingress.NewSimulatorJSONLReplay(db, eventlog.NewEventLog(db), ingress.SimulatorOptions{}, path, "test-sim")
	if _, err := replay.Run(context.Background()); err == nil {
		t.Fatal("expected unknown record rejection")
	}
}
