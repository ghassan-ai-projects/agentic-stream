package eventlog_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/spectest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func registerSchema(t *testing.T, db *storage.DB, ref string) {
	t.Helper()
	definition, ok := spec.LookupEventSchema(ref)
	if !ok {
		t.Fatalf("schema %q is not registered in the built-in catalog", ref)
	}
	schemaJSON, err := spectest.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return spectest.RegisterEventSchema(context.Background(), tx, definition, schemaJSON, "2026-08-12T12:00:00Z")
	})
	if err != nil {
		t.Fatal(err)
	}
}

func temperatureEnvelope(data map[string]any) contractsv1.Envelope {
	event := envelope("evt", "default")
	event.Data = data
	return event
}

func zoneEnvelope(id, eventType string, data map[string]any) contractsv1.Envelope {
	event := envelope(id, "default")
	event.Type, event.PartitionKey = eventType, "zone-01"
	event.Entity = contractsv1.EntityRef{Type: "thermal_zone", ID: "zone-01"}
	event.Data = data
	return event
}

func TestAppendUnderRequiredSchemasAppliesTheRegisteredBuiltInCatalog(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ref   string
		event contractsv1.Envelope
		valid bool
	}{
		{"temperature with an undeclared field and wrong type", "sensor.temperature/1.0", temperatureEnvelope(map[string]any{"value": "hot", "unexpected": true}), false},
		{"temperature payload", "sensor.temperature/1.0", temperatureEnvelope(map[string]any{"value": 42.0}), true},
		{"thermal quality inside the enum", "zone.temp.observed/1.0", zoneEnvelope("evt", "zone.temp.observed", map[string]any{"quality": "valid", "boot_id": "boot-A", "seq": 1.0, "device_mono_us": 1000.0}), true},
		{"thermal quality outside the enum", "zone.temp.observed/1.0", zoneEnvelope("evt", "zone.temp.observed", map[string]any{"quality": "calibrating"}), false},
		{"humidity payload", "zone.humidity.observed/1.0", zoneEnvelope("evt", "zone.humidity.observed", map[string]any{"percent": 46.0, "quality": "valid", "boot_id": "boot-01", "seq": 1, "device_mono_us": 1_234_567}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log, db := newLog(t)
			registerSchema(t, db, tc.ref)
			log.RequireSchemaValidation()

			positions, err := log.Append(t.Context(), "default", []contractsv1.Envelope{tc.event})
			if tc.valid && (err != nil || len(positions) != 1 || positions[0] <= 0) {
				t.Fatalf("conforming event refused: positions=%v err=%v", positions, err)
			}
			if !tc.valid && err == nil {
				t.Fatal("non-conforming event was admitted")
			}
			if !tc.valid && positionOf(t, log) != 0 {
				t.Fatal("a refused event entered the log")
			}
		})
	}
}

func TestValidateEnvelopeChecksTheDurableRegistryOnlyWhenRequired(t *testing.T) {
	t.Parallel()
	log, db := newLog(t)
	event := temperatureEnvelope(map[string]any{"value": "hot"})
	if err := log.ValidateEnvelope(t.Context(), event); err != nil {
		t.Fatalf("validation ran before it was required: %v", err)
	}
	registerSchema(t, db, "sensor.temperature/1.0")
	log.RequireSchemaValidation()
	if err := log.ValidateEnvelope(t.Context(), event); err == nil {
		t.Fatal("a payload of the wrong type was accepted")
	}
}

func positionOf(t *testing.T, log *eventlog.EventLog) eventlog.LogPosition {
	t.Helper()
	position, err := log.CurrentPosition(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	return position
}
