package eventlog_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func registerTemperatureSchema(t *testing.T, db *storage.DB) {
	t.Helper()
	definition, ok := eventschema.Lookup("sensor.temperature/1.0")
	if !ok {
		t.Fatal("temperature schema is not registered in the built-in catalog")
	}
	schemaJSON, err := eventschema.JSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		return eventschema.Register(context.Background(), tx, definition, schemaJSON, "2026-08-12T12:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRejectsUnknownAndWrongTypedPayloadsAgainstDurableSchema(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	registerTemperatureSchema(t, db)
	log := eventlog.NewEventLog(db).RequireSchemaValidation()
	env := contractsv1.Envelope{
		ID: "evt-invalid", Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default", Source: "test",
		PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"value": "hot", "unexpected": true},
	}
	if _, err := log.Append(ctx, "default", []contractsv1.Envelope{env}); err == nil {
		t.Fatal("expected durable schema rejection")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_log").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid event entered event log: %d rows", count)
	}
}

func TestReleasedQuarantineCanBeValidatedAndRedrivenOnce(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "redrive.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	registerTemperatureSchema(t, db)
	log := eventlog.NewEventLog(db).RequireSchemaValidation()
	env := contractsv1.Envelope{
		ID: "evt-redrive", Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default", Source: "test",
		PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"value": 42.0},
	}
	if err := log.QuarantineEnvelope(ctx, "default", env, "operator_hold", "2026-08-12T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := log.ReleaseQuarantine(ctx, "default", env.ID, "2026-08-12T12:01:00Z"); err != nil {
		t.Fatal(err)
	}
	position, err := log.RedriveQuarantine(ctx, "default", env.ID, "2026-08-12T12:02:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if position <= 0 {
		t.Fatalf("redrive position = %d", position)
	}
	var redrivenAt string
	if err := db.QueryRowContext(ctx, "SELECT redriven_at FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", "default", env.ID).Scan(&redrivenAt); err != nil {
		t.Fatal(err)
	}
	if redrivenAt == "" {
		t.Fatal("redrive timestamp was not persisted")
	}
	if _, err := log.RedriveQuarantine(ctx, "default", env.ID, "2026-08-12T12:03:00Z"); err == nil {
		t.Fatal("expected second redrive to fail")
	}
}
