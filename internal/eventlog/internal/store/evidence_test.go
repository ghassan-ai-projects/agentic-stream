package store

import (
	"slices"
	"strings"
	"testing"
)

func TestEvidenceEventsLocateOnlyTheTenantsNamedEventsInLogOrder(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	for _, id := range []string{"evt-1", "evt-2", "evt-3"} {
		appendOne(t, st, validEnvelope(id))
	}
	other := validEnvelope("evt-1")
	other.TenantID = "other"
	appendOne(t, st, other)

	events, err := st.EvidenceEvents(t.Context(), "tenant", []string{"evt-3", "evt-1", "never-logged"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	if !slices.Equal(ids, []string{"evt-1", "evt-3"}) {
		t.Fatalf("ids = %v, want evt-1 then evt-3: log order, no unknown id, no other tenant", ids)
	}
	first := events[0]
	if first.Position <= 0 || first.EventType != "sensor.temperature" || first.Source != "test" || first.EntityID != "motor-1" || first.EventTime == "" || first.IngestedAt == "" {
		t.Fatalf("event = %+v, want position, type, source, entity and times", first)
	}
}

func TestEvidenceEventsOfNoIDsIsEmpty(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	events, err := st.EvidenceEvents(t.Context(), "tenant", nil)
	if err != nil || len(events) != 0 {
		t.Fatalf("events = %+v err=%v, want none", events, err)
	}
}

func TestEvidenceEventsNameAQueryFailure(t *testing.T) {
	t.Parallel()
	st := newStoreWithoutTable(t, "event_log")
	if _, err := st.EvidenceEvents(t.Context(), "tenant", []string{"evt-1"}); err == nil || !strings.Contains(err.Error(), "read evidence events") {
		t.Fatalf("err = %v, want read evidence events", err)
	}
}
