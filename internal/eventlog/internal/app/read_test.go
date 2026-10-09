package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestReadRebuildsTheFullEnvelopeOfEachRecord(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	observed := eventTime.Add(time.Second)
	ingested := eventTime.Add(2 * time.Second)
	full := envelope("evt-full")
	full.ObservedAt = &observed
	full.CorrelationID, full.CausationID = "corr-1", "cause-1"
	full.Traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	full.Tracestate = "vendor=value"
	full.IngestedAt = ingested
	full.Quality = []contractsv1.QualityFlag{{Code: "late"}}
	h.append(t, full, envelope("evt-bare"))

	records := h.readAll(t)
	got := records[0].Envelope
	if got.ID != "evt-full" || got.Data["celsius"] != float64(30) || got.CorrelationID != "corr-1" || got.CausationID != "cause-1" ||
		got.Traceparent != full.Traceparent || got.Tracestate != "vendor=value" || got.Classification != "internal" ||
		got.ObservedAt == nil || !got.ObservedAt.Equal(observed) || len(got.Quality) != 1 || got.Quality[0].Code != "late" {
		t.Fatalf("envelope was not rebuilt from the record: %+v", got)
	}
	bare := records[1]
	if bare.Envelope.CorrelationID != "" || bare.Envelope.Traceparent != "" || bare.ObservedAt != nil {
		t.Fatalf("absent optional fields must read back empty: %+v", bare)
	}
	if bare.Position <= records[0].Position || bare.TenantID != "tenant" || bare.EntityType != "motor" || bare.EntityID != "motor-1" || bare.PartitionKey != "motor-1" || !bare.EventTime.Equal(eventTime) {
		t.Fatalf("record header = %+v", bare)
	}
}

func TestReadRefusesAStoredDocumentItCannotDecode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		column, value, want string
	}{
		{"payload_json", "X'5B5D'", "unmarshal payload"},
		{"quality_json", "X'7B7D'", "unmarshal quality"},
	}
	for _, tc := range cases {
		t.Run(tc.column, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.append(t, envelope("evt-1"))
			h.exec(t, "UPDATE event_log SET "+tc.column+" = "+tc.value)
			err := h.service.Read(t.Context(), readAllRequest, func(Record) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadStopsAtTheVisitorFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"), envelope("evt-2"))
	stopped := errors.New("consumer stopped")
	calls := 0
	err := h.service.Read(t.Context(), readAllRequest, func(Record) error { calls++; return stopped })
	if !errors.Is(err, stopped) || calls != 1 {
		t.Fatalf("err = %v calls = %d, want the visitor error after one call", err, calls)
	}
}
