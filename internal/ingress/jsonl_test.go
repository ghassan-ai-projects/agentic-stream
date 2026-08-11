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

func TestJSONLReplayAppendsEvents(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	tracePath := filepath.Join(dir, "trace.jsonl")
	if err := os.WriteFile(tracePath, []byte(`
{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"motor-17","entity":{"type":"motor","id":"motor-17"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}
{"id":"evt-2","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"motor-17","entity":{"type":"motor","id":"motor-17"},"event_time":"2026-01-01T00:01:00Z","ingested_at":"2026-01-01T00:01:01Z","classification":"internal","data":{"rms_mm_s":6.0}}
`), 0o600); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	log := eventlog.NewEventLog(db)
	conn := ingress.NewJSONLReplay(db, log, "default", tracePath, "test-connector")
	count, err := conn.Run(ctx)
	if err != nil {
		t.Fatalf("run connector: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 appended events, got %d", count)
	}

	// Running again should append nothing because the checkpoint advanced.
	count, err = conn.Run(ctx)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 events on replay, got %d", count)
	}
}

func TestJSONLReplayFillsMissingTenantID(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	tracePath := filepath.Join(dir, "trace.jsonl")
	if err := os.WriteFile(tracePath, []byte(`
{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","source":"sim","partition_key":"motor-17","entity":{"type":"motor","id":"motor-17"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}
`), 0o600); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	log := eventlog.NewEventLog(db)
	conn := ingress.NewJSONLReplay(db, log, "default", tracePath, "")
	if _, err := conn.Run(ctx); err != nil {
		t.Fatalf("run connector: %v", err)
	}

	var tenantID string
	if err := db.QueryRowContext(ctx, "SELECT tenant_id FROM event_log WHERE event_id = ?", "evt-1").Scan(&tenantID); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if tenantID != "default" {
		t.Fatalf("expected tenant default, got %s", tenantID)
	}
}
