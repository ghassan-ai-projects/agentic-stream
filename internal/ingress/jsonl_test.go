package ingress_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
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

func TestJSONLReplayQuarantinesMalformedAndSchemaInvalidLines(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tracePath := filepath.Join(dir, "trace.jsonl")
	contents := "not-json\n" +
		`{"id":"evt-bad","type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"motor-17","entity":{"type":"motor","id":"motor-17"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"unknown":5}}` + "\n"
	if err := os.WriteFile(tracePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewEventLog(db).RequireSchemaValidation()
	// The deployment path normally registers schemas. This test registers the
	// built-in schema directly to exercise the connector boundary in isolation.
	definition, ok := eventschema.Lookup("motor.vibration.observed/1.0")
	if !ok {
		t.Fatal("vibration schema is not registered in the built-in catalog")
	}
	schemaJSON, err := eventschema.JSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return eventschema.Register(ctx, tx, definition, schemaJSON, "2026-08-12T12:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	conn := ingress.NewJSONLReplay(db, log, "default", tracePath, "malformed-test")
	count, err := conn.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("quarantined lines appended %d events", count)
	}
	var quarantined int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_quarantine WHERE tenant_id = 'default'").Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if quarantined != 2 {
		t.Fatalf("quarantined rows = %d, want 2", quarantined)
	}
	var raw string
	if err := db.QueryRowContext(ctx, "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = 'malformed_json'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "not-json" {
		t.Fatalf("raw quarantine payload = %q", raw)
	}
}
