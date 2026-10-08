package eventlog_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAppendRollsBackEarlierRecordsWhenLaterAdmissionFails(t *testing.T) {
	t.Parallel()
	log, cleanup := newTestLog(t)
	t.Cleanup(cleanup)
	first := boundaryEnvelope("first", "tenant")
	invalid := boundaryEnvelope("invalid", "other-tenant")
	positions, err := log.Append(t.Context(), "tenant", []contractsv1.Envelope{first, invalid})
	if err == nil || !strings.Contains(err.Error(), "tenant mismatch") || positions != nil {
		t.Fatalf("batch admission: positions=%v err=%v", positions, err)
	}
	if position, err := log.CurrentPosition(t.Context(), "tenant"); err != nil || position != 0 {
		t.Fatalf("failed batch left committed evidence: position=%d err=%v", position, err)
	}
	positions, err = log.Append(t.Context(), "tenant", []contractsv1.Envelope{first})
	if err != nil || len(positions) != 1 || positions[0] <= 0 {
		t.Fatalf("valid retry was suppressed: positions=%v err=%v", positions, err)
	}
}

func TestAppendValidatesTraceBeforeIgnoringDuplicate(t *testing.T) {
	t.Parallel()
	log, cleanup := newTestLog(t)
	t.Cleanup(cleanup)
	event := boundaryEnvelope("first", "tenant")
	if _, err := log.Append(t.Context(), "tenant", []contractsv1.Envelope{event}); err != nil {
		t.Fatal(err)
	}
	event.Traceparent = "invalid"
	if _, err := log.Append(t.Context(), "tenant", []contractsv1.Envelope{event}); err == nil || !strings.Contains(err.Error(), "validate trace context") {
		t.Fatalf("duplicate bypassed trace validation: %v", err)
	}
}

func TestReadFiltersBeforeLimitAndPreservesLogOrder(t *testing.T) {
	t.Parallel()
	log, cleanup := newTestLog(t)
	t.Cleanup(cleanup)
	first := boundaryEnvelope("first", "tenant")
	second := boundaryEnvelope("second", "tenant")
	second.PartitionKey = "motor-18"
	if first.PartitionID(0) == second.PartitionID(0) {
		t.Fatal("fixture requires distinct partitions")
	}
	third := boundaryEnvelope("third", "tenant")
	positions, err := log.Append(t.Context(), "tenant", []contractsv1.Envelope{first, second, third})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(t.Context(), "other", []contractsv1.Envelope{boundaryEnvelope("other", "other")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		req  eventlog.ReadRequest
		want []string
	}{
		{"default limit", eventlog.ReadRequest{TenantID: "tenant", PartitionID: -1}, []string{"first", "second", "third"}},
		{"negative limit", eventlog.ReadRequest{TenantID: "tenant", PartitionID: -1, Limit: -1}, []string{"first", "second", "third"}},
		{"after position", eventlog.ReadRequest{TenantID: "tenant", PartitionID: -1, AfterPosition: positions[0], Limit: 1}, []string{"second"}},
		{"partition filter", eventlog.ReadRequest{TenantID: "tenant", PartitionID: first.PartitionID(0), AfterPosition: positions[0], Limit: 1}, []string{"third"}},
		{"other tenant", eventlog.ReadRequest{TenantID: "other", PartitionID: -1}, []string{"other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []string
			err := log.Read(t.Context(), tt.req, func(rec eventlog.Record) error { ids = append(ids, rec.EventID); return nil })
			if err != nil || !slices.Equal(ids, tt.want) {
				t.Fatalf("read: ids=%v want=%v err=%v", ids, tt.want, err)
			}
		})
	}
}

func TestReadStopsOnCallbackFailureAndReleasesConnection(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	db.SetMaxOpenConns(1)
	log := eventlog.NewEventLog(db)
	if _, err := log.Append(t.Context(), "tenant", []contractsv1.Envelope{boundaryEnvelope("first", "tenant"), boundaryEnvelope("second", "tenant")}); err != nil {
		t.Fatal(err)
	}
	stopped := errors.New("consumer stopped")
	calls := 0
	err := log.Read(t.Context(), eventlog.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(eventlog.Record) error { calls++; return stopped })
	if !errors.Is(err, stopped) || calls != 1 {
		t.Fatalf("callback failure: err=%v calls=%d", err, calls)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := log.CurrentPosition(ctx, "tenant"); err != nil {
		t.Fatalf("callback failure left the reader connection occupied: %v", err)
	}
}

func boundaryEnvelope(id, tenant string) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: tenant,
		Source: "test", PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 42.0},
	}
}
