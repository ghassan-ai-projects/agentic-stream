package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newTestLog(t *testing.T) (*eventlog.EventLog, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return eventlog.NewEventLog(db), func() { _ = db.Close() }
}

func TestAppendAndRead(t *testing.T) {
	ctx := context.Background()
	log, cleanup := newTestLog(t)
	defer cleanup()

	env := contractsv1.Envelope{
		ID:             "evt-1",
		Type:           "sensor.temperature",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Traceparent:    "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Tracestate:     "vendor=value",
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"celsius": 42.0},
	}

	positions, err := log.Append(ctx, "default", []contractsv1.Envelope{env})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if positions[0] <= 0 {
		t.Fatalf("unexpected position %d", positions[0])
	}

	partitionID := env.PartitionID(0)
	var records []eventlog.Record
	err = log.Read(ctx, eventlog.ReadRequest{
		TenantID:      "default",
		PartitionID:   partitionID,
		AfterPosition: 0,
		Limit:         10,
	}, func(r eventlog.Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].EventID != "evt-1" {
		t.Fatalf("unexpected event id %q", records[0].EventID)
	}
	if records[0].Envelope.Data["celsius"] != 42.0 {
		t.Fatalf("unexpected payload %v", records[0].Envelope.Data)
	}
	if records[0].Envelope.Traceparent != env.Traceparent || records[0].Envelope.Tracestate != env.Tracestate {
		t.Fatalf("trace context was not persisted: parent=%q state=%q", records[0].Envelope.Traceparent, records[0].Envelope.Tracestate)
	}
}

func TestCurrentPositionIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	log, cleanup := newTestLog(t)
	defer cleanup()

	base := contractsv1.Envelope{
		ID:             "evt-default-1",
		Type:           "sensor.temperature",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"celsius": 42.0},
	}
	position, err := log.Append(ctx, "default", []contractsv1.Envelope{base})
	if err != nil {
		t.Fatalf("append default event: %v", err)
	}

	other := base
	other.ID = "evt-other-1"
	other.TenantID = "other"
	other.PartitionKey = "motor-18"
	other.Entity = contractsv1.EntityRef{Type: "motor", ID: "motor-18"}
	otherPosition, err := log.Append(ctx, "other", []contractsv1.Envelope{other})
	if err != nil {
		t.Fatalf("append other event: %v", err)
	}

	latest := base
	latest.ID = "evt-default-2"
	latest.EventTime = latest.EventTime.Add(time.Second)
	latest.IngestedAt = latest.IngestedAt.Add(time.Second)
	latestPosition, err := log.Append(ctx, "default", []contractsv1.Envelope{latest})
	if err != nil {
		t.Fatalf("append latest default event: %v", err)
	}

	if got, err := log.CurrentPosition(ctx, "missing"); err != nil {
		t.Fatalf("current position for empty tenant: %v", err)
	} else if got != 0 {
		t.Fatalf("empty tenant position = %d, want 0", got)
	}
	if got, err := log.CurrentPosition(ctx, "default"); err != nil {
		t.Fatalf("current default position: %v", err)
	} else if got != latestPosition[0] {
		t.Fatalf("default position = %d, want %d", got, latestPosition[0])
	}
	if got, err := log.CurrentPosition(ctx, "other"); err != nil {
		t.Fatalf("current other position: %v", err)
	} else if got != otherPosition[0] {
		t.Fatalf("other position = %d, want %d", got, otherPosition[0])
	}
	if position[0] >= latestPosition[0] {
		t.Fatalf("positions did not advance: first=%d latest=%d", position[0], latestPosition[0])
	}
}

func TestAppendDuplicateIgnored(t *testing.T) {
	ctx := context.Background()
	log, cleanup := newTestLog(t)
	defer cleanup()

	env := contractsv1.Envelope{
		ID:             "evt-1",
		Type:           "sensor.temperature",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"celsius": 42.0},
	}

	_, err := log.Append(ctx, "default", []contractsv1.Envelope{env})
	if err != nil {
		t.Fatalf("first Append failed: %v", err)
	}
	positions, err := log.Append(ctx, "default", []contractsv1.Envelope{env})
	if err != nil {
		t.Fatalf("second Append failed: %v", err)
	}
	if positions[0] != -1 {
		t.Fatalf("expected duplicate position -1, got %d", positions[0])
	}
}
