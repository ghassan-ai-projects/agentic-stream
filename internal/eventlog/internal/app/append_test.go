package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

func TestAppendRejectsInvalidEnvelopeAndRollsBackEarlierRecords(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	invalid := envelope("evt-bad")
	invalid.Type = ""
	_, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{envelope("evt-1"), invalid})
	if err == nil || !strings.Contains(err.Error(), "append events: validate envelope") {
		t.Fatalf("err = %v, want append events: validate envelope", err)
	}
	if position, err := h.service.CurrentPosition(t.Context(), "tenant"); err != nil || position != 0 {
		t.Fatalf("earlier records were not rolled back: position=%d err=%v", position, err)
	}
	if positions := h.append(t, envelope("evt-1")); len(positions) != 1 || positions[0] <= 0 {
		t.Fatalf("the valid event could not be appended again after the rollback: %v", positions)
	}
}

func TestAppendRefusesAnEnvelopeOfAnotherTenant(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	foreign := envelope("evt-foreign")
	foreign.TenantID = "other"
	if _, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{foreign}); err == nil || !strings.Contains(err.Error(), "tenant mismatch") {
		t.Fatalf("err = %v, want tenant mismatch", err)
	}
}

func TestAppendReportsDuplicatesAsMinusOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	positions := h.append(t, envelope("evt-1"), envelope("evt-1"))
	if positions[0] <= 0 || positions[1] != -1 {
		t.Fatalf("positions = %v, want a positive position then -1", positions)
	}
	redelivered := h.append(t, envelope("evt-1"), envelope("evt-2"))
	if redelivered[0] != -1 || redelivered[1] <= positions[0] {
		t.Fatalf("redelivery positions = %v, want -1 then a new position", redelivered)
	}
	if got := len(h.readAll(t)); got != 2 {
		t.Fatalf("log holds %d records, want 2", got)
	}
}

func TestAppendValidatesTraceContextEvenForADuplicate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	event := envelope("evt-1")
	h.append(t, event)
	event.Traceparent = "not-a-traceparent"
	if _, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{event}); err == nil || !strings.Contains(err.Error(), "validate trace context") {
		t.Fatalf("err = %v, want validate trace context: a duplicate must not bypass the check", err)
	}
}

func TestOutOfOrderEventsAreLoggedInArrivalOrderWithTheirOwnEventTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	onTime, late := envelope("on-time"), envelope("late")
	onTime.EventTime = eventTime.Add(10 * time.Minute)
	late.EventTime = eventTime
	positions := h.append(t, onTime, late)
	if positions[1] <= positions[0] {
		t.Fatalf("positions = %v, want the late event after the on-time event", positions)
	}
	records := h.readAll(t)
	if !records[0].EventTime.Equal(onTime.EventTime) || !records[1].EventTime.Equal(late.EventTime) {
		t.Fatalf("event times = %v, %v, want each event's own", records[0].EventTime, records[1].EventTime)
	}
	if records[0].EventID != "on-time" || records[1].EventID != "late" {
		t.Fatalf("log order = %s, %s, want arrival order", records[0].EventID, records[1].EventID)
	}
}

func TestAppendStampsTheRecordWithTheInjectedClock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"))
	var createdAt string
	if err := h.db.QueryRowContext(t.Context(), "SELECT created_at FROM event_log").Scan(&createdAt); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(createdAt, "2026-08-12T12:00:00") {
		t.Fatalf("created_at = %q, want the virtual clock's 2026-08-12T12:00:00", createdAt)
	}
}

func TestAppendUnderRequiredSchemasAdmitsOnlyConformingPayloads(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.registerTemperatureSchema(t, temperatureSchema)
	h.service.RequireSchemaValidation()
	for name, data := range map[string]map[string]any{
		"undeclared field": {"celsius": 30, "extra": true},
		"wrong type":       {"celsius": "hot"},
		"missing required": {},
	} {
		bad := envelope("evt-bad")
		bad.Data = data
		if _, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{envelope("evt-good"), bad}); err == nil || !strings.Contains(err.Error(), "payload field") {
			t.Errorf("%s: err = %v, want a payload field rejection", name, err)
		}
	}
	if records := h.readAll(t); len(records) != 0 {
		t.Fatalf("a rejected batch left %d records, want none", len(records))
	}
	if positions := h.append(t, envelope("evt-good")); positions[0] <= 0 {
		t.Fatalf("conforming payload refused: %v", positions)
	}
}

func TestAppendUnderRequiredSchemasRefusesAnUnregisteredOrCorruptSchema(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.service.RequireSchemaValidation()
	if _, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{envelope("evt-1")}); err == nil || !strings.Contains(err.Error(), "is not registered") {
		t.Fatalf("err = %v, want is not registered", err)
	}
	h.registerTemperatureSchema(t, "not json")
	if _, err := h.service.Append(t.Context(), "tenant", []contractsv1.Envelope{envelope("evt-1")}); err == nil || !strings.Contains(err.Error(), "decode event schema") {
		t.Fatalf("err = %v, want decode event schema", err)
	}
}

func TestValidateEnvelopeIsOptionalUntilSchemasAreRequired(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.service.ValidateEnvelope(t.Context(), envelope("evt-1")); err != nil {
		t.Fatalf("validation ran before it was required: %v", err)
	}
	h.service.RequireSchemaValidation()
	if err := h.service.ValidateEnvelope(t.Context(), envelope("evt-1")); err == nil || !strings.Contains(err.Error(), "is not registered") {
		t.Fatalf("err = %v, want is not registered", err)
	}
	h.registerTemperatureSchema(t, temperatureSchema)
	if err := h.service.ValidateEnvelope(t.Context(), envelope("evt-1")); err != nil {
		t.Fatalf("conforming envelope refused: %v", err)
	}
}

func TestReadEntityEventsWrapsTheWindowRead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"), envelope("evt-2"))
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: eventTime, Until: eventTime, MaxRows: 10}
	var ids []string
	err := h.service.ReadEntityEvents(t.Context(), window, func(e domain.EntityEvent) (bool, error) {
		ids = append(ids, e.EventID)
		return true, nil
	})
	if err != nil || !slices.Equal(ids, []string{"evt-1", "evt-2"}) {
		t.Fatalf("ids = %v err=%v, want evt-1, evt-2", ids, err)
	}
}

func TestEveryOperationNamesItsFailureWhenStorageIsGone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		call func(context.Context, *Service) error
		want string
	}{
		{"append", func(ctx context.Context, s *Service) error {
			_, err := s.Append(ctx, "tenant", []contractsv1.Envelope{envelope("evt-1")})
			return err
		}, "append events"},
		{"current position", func(ctx context.Context, s *Service) error { _, err := s.CurrentPosition(ctx, "tenant"); return err }, "read current event position"},
		{"evidence events", func(ctx context.Context, s *Service) error {
			_, err := s.EvidenceEvents(ctx, "tenant", []string{"evt-1"})
			return err
		}, "evidence events of tenant tenant"},
		{"quarantined", func(ctx context.Context, s *Service) error { _, err := s.Quarantined(ctx, "tenant"); return err }, "quarantined records of tenant tenant"},
		{"quarantine", func(ctx context.Context, s *Service) error {
			return s.Quarantine(ctx, "tenant", map[string]any{"id": "bad"}, "reason", noon)
		}, "quarantine event transaction"},
		{"release", func(ctx context.Context, s *Service) error { return s.ReleaseQuarantine(ctx, "tenant", "bad", noon) }, "release quarantine transaction"},
		{"redrive", func(ctx context.Context, s *Service) error {
			_, err := s.RedriveQuarantine(ctx, "tenant", "bad", noon)
			return err
		}, "redrive quarantine transaction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			if err := h.db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := tc.call(t.Context(), h.service); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestAReusedEventIDWithADifferentPayloadIsQuarantinedNotSilentlyDropped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"))
	redelivered := h.append(t, envelope("evt-1"))
	reused := envelope("evt-1")
	reused.Data = map[string]any{"celsius": 99}
	conflicting := h.append(t, reused)
	if redelivered[0] != -1 || conflicting[0] != -1 {
		t.Fatalf("positions = %v and %v, want both refused as already logged", redelivered, conflicting)
	}
	var reason string
	if err := h.db.QueryRowContext(t.Context(), "SELECT reason_code FROM event_quarantine WHERE tenant_id = 'tenant' AND event_id = 'evt-1'").Scan(&reason); err != nil || reason != domain.ReasonEventIDConflict {
		t.Fatalf("quarantine reason = %q err=%v, want %s", reason, err, domain.ReasonEventIDConflict)
	}
	var quarantined int
	if err := h.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_quarantine").Scan(&quarantined); err != nil || quarantined != 1 {
		t.Fatalf("quarantined = %d err=%v, want only the conflicting delivery", quarantined, err)
	}
}
