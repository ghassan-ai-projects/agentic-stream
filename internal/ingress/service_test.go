package ingress_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

const vibrationLine = `{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"m1","entity":{"type":"motor","id":"m1"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}`

func newService(t *testing.T) (*ingress.Service, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	service, err := ingress.New(ingress.Config{DB: db, Log: eventlog.NewEventLog(db), TenantID: "default", Telemetry: telemetry.NewRuntime(time.Unix(1, 0))})
	if err != nil {
		t.Fatal(err)
	}
	return service, db
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	tests := []struct {
		name   string
		config ingress.Config
	}{
		{name: "no database", config: ingress.Config{Log: eventlog.NewEventLog(db)}},
		{name: "no event log", config: ingress.Config{DB: db}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service, err := ingress.New(tt.config)
			if err == nil || !strings.Contains(err.Error(), "requires a database and an event log") || service != nil {
				t.Fatalf("service = %v, err = %v, want a refusal naming the missing dependency", service, err)
			}
		})
	}
}

func TestServiceReplaysJSONLAndResumesThroughTheFacade(t *testing.T) {
	t.Parallel()
	service, _ := newService(t)
	path := writeFile(t, "trace.jsonl", vibrationLine+"\n")

	if count, err := service.ReplayJSONL(t.Context(), path, ""); err != nil || count != 1 {
		t.Fatalf("first replay count = %d, err = %v", count, err)
	}
	if count, err := service.ReplayJSONL(t.Context(), path, ""); err != nil || count != 0 {
		t.Fatalf("a resumed replay must append nothing: count = %d, err = %v", count, err)
	}
}

func TestServiceReplaysASimulatorTraceThroughTheFacade(t *testing.T) {
	t.Parallel()
	service, db := newService(t)
	path := writeFile(t, "simulator.jsonl", strings.Join([]string{
		`{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}`,
		`{"record_type":"event","event":{"id":"evt-pump-1","entity_type":"pump","entity_id":"pump-1","type":"vibration","event_time":"2026-07-29T09:00:00Z","arrival_time":"2026-07-29T09:00:01Z","value":5.2}}`,
		`{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}`,
	}, "\n")+"\n")
	options := ingress.SimulatorOptions{TenantID: "default", EntityType: "pump"}

	count, err := service.ReplaySimulator(t.Context(), options, path, "")
	if err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want the one event", count, err)
	}
	if count, err := service.ReplaySimulator(t.Context(), options, path, ""); err != nil || count != 0 {
		t.Fatalf("a resumed replay must append nothing: count = %d, err = %v", count, err)
	}

	var eventType string
	if err := db.QueryRowContext(t.Context(), "SELECT event_type FROM event_log WHERE event_id = 'evt-pump-1'").Scan(&eventType); err != nil || eventType != "pump.vibration.observed" {
		t.Fatalf("event type = %q, err = %v, want pump.vibration.observed", eventType, err)
	}
}

func TestServiceRefusesALiveSourceWithoutASink(t *testing.T) {
	t.Parallel()
	service, _ := newService(t)

	err := service.ServeLive(t.Context(), "/tmp/never-bound.sock", nil)

	if err == nil || !strings.Contains(err.Error(), "sink is required") {
		t.Fatalf("err = %v, want the missing sink refusal", err)
	}
}
