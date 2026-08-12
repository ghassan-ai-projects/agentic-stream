package ingress_test

import (
	"context"
	"os"
	"path/filepath"
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
{"record_type":"event","event.id":"evt-000001","event.entity_type":"pump","event.entity_id":"site-a.pump-1","event.type":"vibration","event.event_time":"2026-07-29T09:00:00Z","event.arrival_time":"2026-07-29T09:00:01.5Z","event.value":5.2,"event.unit":"mm/s"}
{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	replay := ingress.NewSimulatorJSONLReplay(db, eventlog.NewEventLog(db), ingress.SimulatorOptions{TenantID: "default"}, path, "test-sim")
	count, err := replay.Run(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var typ, entity, payload string
	if err := db.QueryRowContext(context.Background(), "SELECT event_type, entity_type, payload_json FROM event_log").Scan(&typ, &entity, &payload); err != nil {
		t.Fatal(err)
	}
	if typ != "motor.vibration.observed" || entity != "motor" || payload == "" {
		t.Fatalf("unexpected event type=%q entity=%q payload=%q", typ, entity, payload)
	}
	count, err = replay.Run(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("repeat count=%d err=%v", count, err)
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
