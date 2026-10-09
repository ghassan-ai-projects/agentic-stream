package eventlog_test

import (
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	eventTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	noon      = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
)

func newLog(t *testing.T) (*eventlog.EventLog, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return eventlog.NewEventLog(db), db
}

func envelope(id, tenant string) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: tenant,
		Source: "test", PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime: eventTime, IngestedAt: eventTime.Add(time.Second),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 42.0},
	}
}

func readIDs(t *testing.T, log *eventlog.EventLog, req eventlog.ReadRequest) []string {
	t.Helper()
	var ids []string
	err := log.Read(t.Context(), req, func(rec eventlog.Record) error { ids = append(ids, rec.EventID); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestAppendedEventsReadBackInLogOrderWithTheirTraceContext(t *testing.T) {
	t.Parallel()
	log, _ := newLog(t)
	first := envelope("evt-1", "default")
	first.Traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	first.Tracestate = "vendor=value"
	positions, err := log.Append(t.Context(), "default", []contractsv1.Envelope{first, envelope("evt-2", "default")})
	if err != nil || len(positions) != 2 || positions[0] <= 0 || positions[1] <= positions[0] {
		t.Fatalf("positions = %v err=%v, want two increasing positions", positions, err)
	}

	var records []eventlog.Record
	req := eventlog.ReadRequest{TenantID: "default", PartitionID: first.PartitionID(0), Limit: 10}
	if err := log.Read(t.Context(), req, func(r eventlog.Record) error { records = append(records, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].EventID != "evt-1" || records[1].EventID != "evt-2" || records[0].Position != positions[0] {
		t.Fatalf("records = %+v, want evt-1 then evt-2 at the appended positions", records)
	}
	if records[0].Envelope.Data["celsius"] != 42.0 || records[0].Envelope.Traceparent != first.Traceparent || records[0].Envelope.Tracestate != first.Tracestate {
		t.Fatalf("payload or trace context was not persisted: %+v", records[0].Envelope)
	}
}

func TestRedeliveredEventIsReportedAsADuplicateAndNotLoggedTwice(t *testing.T) {
	t.Parallel()
	log, _ := newLog(t)
	event := envelope("evt-1", "default")
	if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{event}); err != nil {
		t.Fatal(err)
	}
	positions, err := log.Append(t.Context(), "default", []contractsv1.Envelope{event})
	if err != nil || positions[0] != -1 {
		t.Fatalf("positions = %v err=%v, want the duplicate position -1", positions, err)
	}
	if ids := readIDs(t, log, eventlog.ReadRequest{TenantID: "default", PartitionID: -1}); !slices.Equal(ids, []string{"evt-1"}) {
		t.Fatalf("log = %v, want evt-1 once", ids)
	}
}

func TestCurrentPositionIsTenantScoped(t *testing.T) {
	t.Parallel()
	log, _ := newLog(t)
	defaultPositions, err := log.Append(t.Context(), "default", []contractsv1.Envelope{envelope("evt-default", "default")})
	if err != nil {
		t.Fatal(err)
	}
	otherPositions, err := log.Append(t.Context(), "other", []contractsv1.Envelope{envelope("evt-other", "other")})
	if err != nil {
		t.Fatal(err)
	}
	for tenant, want := range map[string]eventlog.LogPosition{"default": defaultPositions[0], "other": otherPositions[0], "missing": 0} {
		if got, err := log.CurrentPosition(t.Context(), tenant); err != nil || got != want {
			t.Errorf("CurrentPosition(%q) = %d err=%v, want %d", tenant, got, err, want)
		}
	}
}

func TestEventLogWithoutADatabaseRefusesToReportItsPosition(t *testing.T) {
	t.Parallel()
	var missing *eventlog.EventLog
	if _, err := missing.CurrentPosition(t.Context(), "default"); err == nil || err.Error() != "event log storage is required" {
		t.Fatalf("err = %v, want event log storage is required", err)
	}
}

func TestEventLogStampsRecordsWithItsClock(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	for name, log := range map[string]*eventlog.EventLog{
		"virtual":  eventlog.NewEventLogWithClock(db, sources.NewVirtual(noon)),
		"defaults": eventlog.NewEventLogWithClock(db, nil),
	} {
		if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{envelope("evt-"+name, "default")}); err != nil {
			t.Fatalf("%s clock: %v", name, err)
		}
	}
	var virtualStamp string
	if err := db.QueryRowContext(t.Context(), "SELECT created_at FROM event_log WHERE event_id = 'evt-virtual'").Scan(&virtualStamp); err != nil {
		t.Fatal(err)
	}
	if virtualStamp[:19] != "2026-08-12T12:00:00" {
		t.Fatalf("created_at = %q, want the virtual clock's instant", virtualStamp)
	}
}

func TestEvidenceEventsLocateOnlyLoggedEvents(t *testing.T) {
	t.Parallel()
	log, _ := newLog(t)
	if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{envelope("evt-1", "default")}); err != nil {
		t.Fatal(err)
	}
	events, err := log.EvidenceEvents(t.Context(), "default", []string{"evt-1", "never-logged"})
	if err != nil || len(events) != 1 || events[0].EventID != "evt-1" || events[0].Position <= 0 {
		t.Fatalf("events = %+v err=%v, want evt-1 only", events, err)
	}
}
