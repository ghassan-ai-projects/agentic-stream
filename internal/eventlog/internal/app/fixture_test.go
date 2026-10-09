package app

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	eventTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	noon      = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
)

type harness struct {
	service *Service
	db      *storage.DB
}

func newHarness(t *testing.T) harness {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return harness{service: New(sources.NewVirtual(noon), store.New(db)), db: db}
}

func envelope(id string) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "tenant",
		Source: "test", PartitionKey: "motor-1",
		Entity:    contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: eventTime, IngestedAt: eventTime,
		Classification: "internal", Data: map[string]any{"celsius": 30},
	}
}

func (h harness) append(t *testing.T, envelopes ...contractsv1.Envelope) []domain.LogPosition {
	t.Helper()
	positions, err := h.service.Append(t.Context(), "tenant", envelopes)
	if err != nil {
		t.Fatal(err)
	}
	return positions
}

func (h harness) read(t *testing.T, req domain.ReadRequest) []Record {
	t.Helper()
	var records []Record
	err := h.service.Read(t.Context(), req, func(r Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

var readAllRequest = domain.ReadRequest{TenantID: "tenant", PartitionID: -1}

func (h harness) readAll(t *testing.T) []Record {
	t.Helper()
	return h.read(t, readAllRequest)
}

func (h harness) quarantined(t *testing.T) []domain.QuarantineRecord {
	t.Helper()
	records, err := h.service.Quarantined(t.Context(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func (h harness) exec(t *testing.T, statement string, args ...any) {
	t.Helper()
	if _, err := h.db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

func (h harness) registerTemperatureSchema(t *testing.T, schemaJSON string) {
	t.Helper()
	h.exec(t, `INSERT INTO event_schemas (schema_id, event_type, schema_version, schema_json, schema_sha256, status, created_at)
		VALUES ('schema-1', 'sensor.temperature', '1.0', ?, zeroblob(32), 'active', '2026-08-12T12:00:00Z')`, []byte(schemaJSON))
}

const temperatureSchema = `{"properties":{"celsius":{"type":"number"}},"additionalProperties":false,"required":["celsius"]}`
