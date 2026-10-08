package ingress_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNewRefusesMissingDependencies(t *testing.T) {
	t.Parallel()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "ingress.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if service, err := ingress.New(ingress.Config{Log: eventlog.NewEventLog(db)}); err == nil || service != nil {
		t.Fatal("ingress without a database was constructed")
	}
	if service, err := ingress.New(ingress.Config{DB: db}); err == nil || service != nil {
		t.Fatal("ingress without an event log was constructed")
	}
}

func TestServiceReplaysAndResumesThroughTheFacade(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := storagetest.Open(t.Context(), filepath.Join(dir, "ingress.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service, err := ingress.New(ingress.Config{DB: db, Log: eventlog.NewEventLog(db), TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "trace.jsonl")
	line := `{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"m1","entity":{"type":"motor","id":"m1"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if count, err := service.ReplayJSONL(t.Context(), path, ""); err != nil || count != 1 {
		t.Fatalf("first replay count=%d err=%v", count, err)
	}
	if count, err := service.ReplayJSONL(t.Context(), path, ""); err != nil || count != 0 {
		t.Fatalf("a resumed replay must append nothing: count=%d err=%v", count, err)
	}
	if err := service.ServeLive(t.Context(), "relative.sock", nil); err == nil {
		t.Fatal("a live source without a sink was served")
	}
}
