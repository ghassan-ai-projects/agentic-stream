package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

func TestInsertEventReportsDuplicatesAsMinusOneAndPositionsAdvance(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	first := appendOne(t, st, validEnvelope("evt-1"))
	if first <= 0 {
		t.Fatalf("first insert position = %d, want positive", first)
	}
	if duplicate := appendOne(t, st, validEnvelope("evt-1")); duplicate != -1 {
		t.Fatalf("duplicate insert position = %d, want -1", duplicate)
	}
	second := appendOne(t, st, validEnvelope("evt-2"))
	if second <= first {
		t.Fatalf("positions did not advance: first=%d second=%d", first, second)
	}
}

func TestInsertEventKeepsTheFirstDeliveryOfAnEventIDPerTenant(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	changed := validEnvelope("evt-1")
	changed.Data = map[string]any{"celsius": 99}
	if position := appendOne(t, st, changed); position != -1 {
		t.Fatalf("redelivery with a changed payload position = %d, want -1", position)
	}
	stored := readAll(t, st, domain.ReadRequest{TenantID: "tenant", PartitionID: -1})
	if len(stored) != 1 || string(stored[0].PayloadJSON) != `{"celsius":30}` {
		t.Fatalf("stored = %+v, want the first delivery only", stored)
	}
	otherTenant := validEnvelope("evt-1")
	otherTenant.TenantID = "other"
	if position := appendOne(t, st, otherTenant); position <= 0 {
		t.Fatalf("same event id in another tenant position = %d, want a new record", position)
	}
}

func TestCurrentPositionIsTheTenantsGreatestPosition(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	if position, err := st.CurrentPosition(t.Context(), "tenant"); err != nil || position != 0 {
		t.Fatalf("empty log position = %d err=%v, want 0", position, err)
	}
	appendOne(t, st, validEnvelope("evt-1"))
	other := validEnvelope("evt-other")
	other.TenantID = "other"
	otherPosition := appendOne(t, st, other)
	latest := appendOne(t, st, validEnvelope("evt-2"))
	for tenant, want := range map[string]domain.LogPosition{"tenant": latest, "other": otherPosition, "missing": 0} {
		if got, err := st.CurrentPosition(t.Context(), tenant); err != nil || got != want {
			t.Errorf("CurrentPosition(%q) = %d err=%v, want %d", tenant, got, err, want)
		}
	}
}

func TestInsertEventStoresEveryOptionalEnvelopeField(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	observed := eventTime.Add(-time.Second)
	env := validEnvelope("evt-full")
	env.ObservedAt = &observed
	env.CorrelationID, env.CausationID = "corr-1", "cause-1"
	env.Traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	env.Tracestate = "vendor=value"
	env.Quality = []contractsv1.QualityFlag{{Code: "late"}}
	appendOne(t, st, env)
	appendOne(t, st, validEnvelope("evt-bare"))

	stored := readAll(t, st, domain.ReadRequest{TenantID: "tenant", PartitionID: -1})
	full, bare := stored[0], stored[1]
	if full.ObservedAt == nil || !full.ObservedAt.Equal(observed) || *full.CorrelationID != "corr-1" || *full.CausationID != "cause-1" ||
		*full.Traceparent != env.Traceparent || *full.Tracestate != "vendor=value" || !strings.Contains(string(full.QualityJSON), `"late"`) {
		t.Fatalf("optional fields were not stored: %+v", full)
	}
	if bare.ObservedAt != nil || bare.CorrelationID != nil || bare.CausationID != nil || bare.Traceparent != nil || bare.Tracestate != nil {
		t.Fatalf("absent optional fields must read back as nil: %+v", bare)
	}
}

func TestEventStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	cases := []unitFailure{
		{"insert event", "event_log", func(ctx context.Context, u *Unit) error {
			_, err := u.InsertEvent(ctx, "tenant", validEnvelope("evt-1"), domain.EncodedEvent{}, storedAt)
			return err
		}, "insert event"},
		{"load schema", "event_schemas", func(ctx context.Context, u *Unit) error {
			_, err := u.LoadEventSchemaJSON(ctx, "sensor.temperature", "1.0")
			return err
		}, "event schema sensor.temperature/1.0 is not registered"},
	}
	checkUnitFailures(t, cases)
}
