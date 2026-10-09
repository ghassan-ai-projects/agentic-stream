package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/spectest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

// A reading quarantined because its schema was not registered yet is released
// by an operator and redriven once the schema exists: it enters the log once.
func TestQuarantineReleaseAndRedriveRecoverEvidence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	quarantineReading(t, path)
	steps := []struct {
		args    []string
		want    string
		wantErr bool
	}{
		{args: []string{"quarantine", "list", "--db", path}, want: "evt-q-1\tquarantined\tschema_unregistered"},
		{args: []string{"quarantine", "redrive", "evt-q-1", "--db", path}, wantErr: true},
		{args: []string{"quarantine", "release", "evt-q-1", "--db", path}, want: "quarantine: evt-q-1 released"},
		{args: []string{"quarantine", "redrive", "evt-q-1", "--db", path}, want: "redriven at log position"},
		{args: []string{"quarantine", "list", "--db", path, "--json"}, want: `"status":"redriven"`},
		{args: []string{"quarantine", "redrive", "evt-q-1", "--db", path}, wantErr: true},
	}
	for _, step := range steps {
		out, err := runOperatorCommand(t, step.args...)
		if step.wantErr != (err != nil) || !strings.Contains(out, step.want) {
			t.Fatalf("%v = %q, %v", step.args, out, err)
		}
	}
	if logged := countRows(t, path, "event_log"); logged != 1 {
		t.Fatalf("event log holds %d records, want the redriven reading once", logged)
	}
}

func quarantineReading(t *testing.T, path string) {
	t.Helper()
	db, err := storagetest.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	reading := contractsv1.Envelope{
		ID: "evt-q-1", Type: "zone.temp.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
		PartitionKey: "zone-01", Entity: contractsv1.EntityRef{Type: "thermal_zone", ID: "zone-01"},
		EventTime: time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 10, 8, 10, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 31.5, "quality": "valid"},
	}
	if err := eventlog.NewEventLog(db).QuarantineEnvelope(t.Context(), "default", reading, "schema_unregistered", time.Date(2026, 10, 8, 10, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	registerZoneTemperatureSchema(t, db)
}

func registerZoneTemperatureSchema(t *testing.T, db *storage.DB) {
	t.Helper()
	definition, ok := spec.LookupEventSchema("zone.temp.observed/1.0")
	if !ok {
		t.Fatal("zone.temp.observed/1.0 is not a built-in schema")
	}
	schemaJSON, err := spectest.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return spectest.RegisterEventSchema(t.Context(), tx, definition, schemaJSON, "2026-10-08T10:00:02Z")
	}); err != nil {
		t.Fatal(err)
	}
}
