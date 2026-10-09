package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestReadRecordsScansEveryColumn(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	visited := readAll(t, st, domain.ReadRequest{TenantID: "tenant", PartitionID: -1})
	if len(visited) != 1 {
		t.Fatalf("visited %d records, want 1", len(visited))
	}
	first := visited[0]
	if first.EventID != "evt-1" || first.EntityID != "motor-1" || first.TenantID != "tenant" || first.Source != "test" || first.Classification != "internal" || len(first.PayloadJSON) == 0 {
		t.Fatalf("scanned record is incomplete: %+v", first)
	}
	if !first.EventTime.Equal(eventTime) || !first.IngestedAt.Equal(eventTime) {
		t.Fatalf("times = %v, %v, want %v", first.EventTime, first.IngestedAt, eventTime)
	}
}

func TestReadRecordsFiltersBeforeLimitAndPreservesLogOrder(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	motor1, motor2 := validEnvelope("first"), validEnvelope("second")
	motor2.PartitionKey = "motor-18"
	other := validEnvelope("other")
	other.TenantID = "other"
	positions := []domain.LogPosition{appendOne(t, st, motor1), appendOne(t, st, motor2), appendOne(t, st, validEnvelope("third")), appendOne(t, st, other)}
	partition := motor1.PartitionID(0)
	if partition == motor2.PartitionID(0) {
		t.Fatal("fixture requires distinct partitions")
	}
	cases := []struct {
		name string
		req  domain.ReadRequest
		want []string
	}{
		{"every record of the tenant", domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, []string{"first", "second", "third"}},
		{"negative limit uses the default", domain.ReadRequest{TenantID: "tenant", PartitionID: -1, Limit: -1}, []string{"first", "second", "third"}},
		{"limit cuts the tail", domain.ReadRequest{TenantID: "tenant", PartitionID: -1, Limit: 2}, []string{"first", "second"}},
		{"after a position is exclusive", domain.ReadRequest{TenantID: "tenant", PartitionID: -1, AfterPosition: positions[0], Limit: 1}, []string{"second"}},
		{"partition filter applies before the limit", domain.ReadRequest{TenantID: "tenant", PartitionID: partition, AfterPosition: positions[0], Limit: 1}, []string{"third"}},
		{"another tenant", domain.ReadRequest{TenantID: "other", PartitionID: -1}, []string{"other"}},
		{"unknown partition", domain.ReadRequest{TenantID: "tenant", PartitionID: 99999}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := eventIDs(readAll(t, st, tc.req))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLateArrivingEventKeepsItsEventTimeAndTakesTheNextLogPosition(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	onTime, late := validEnvelope("on-time"), validEnvelope("late")
	onTime.EventTime = eventTime.Add(10 * time.Minute)
	late.EventTime = eventTime
	onTimePosition := appendOne(t, st, onTime)
	latePosition := appendOne(t, st, late)
	if latePosition <= onTimePosition {
		t.Fatalf("late event position %d must follow the on-time position %d", latePosition, onTimePosition)
	}
	records := readAll(t, st, domain.ReadRequest{TenantID: "tenant", PartitionID: -1})
	if got := eventIDs(records); !slices.Equal(got, []string{"on-time", "late"}) {
		t.Fatalf("log order = %v, want arrival order", got)
	}
	if !records[1].EventTime.Equal(eventTime) || !records[0].EventTime.Equal(eventTime.Add(10*time.Minute)) {
		t.Fatalf("event times were rewritten: %v, %v", records[0].EventTime, records[1].EventTime)
	}
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: eventTime, Until: eventTime.Add(time.Hour), MaxRows: 10}
	if got := entityWindowIDs(t, st, window, 10); !slices.Equal(got, []string{"late", "on-time"}) {
		t.Fatalf("entity window order = %v, want event-time order", got)
	}
}

func TestReadRecordsStopsOnCallbackFailureAndReleasesTheConnection(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	db.SetMaxOpenConns(1)
	st := New(db)
	appendOne(t, st, validEnvelope("first"))
	appendOne(t, st, validEnvelope("second"))
	stopped := errors.New("consumer stopped")
	calls := 0
	err := st.ReadRecords(t.Context(), domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(domain.ScannedEvent) error { calls++; return stopped })
	if !errors.Is(err, stopped) || calls != 1 {
		t.Fatalf("callback failure: err=%v calls=%d, want the callback error after one call", err, calls)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := st.CurrentPosition(ctx, "tenant"); err != nil {
		t.Fatalf("callback failure left the only connection occupied: %v", err)
	}
}

func TestReadRecordsRefusesCorruptStoredTimesInColumnOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ column, want string }{
		{"event_time", "parse event_time"}, {"ingested_at", "parse ingested_at"}, {"observed_at", "parse observed_at: parse observed time"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			t.Parallel()
			st := newStore(t)
			appendOne(t, st, validEnvelope("evt-1"))
			if _, err := st.DB.ExecContext(t.Context(), "UPDATE event_log SET "+tc.column+" = 'not a time'"); err != nil {
				t.Fatal(err)
			}
			err := st.ReadRecords(t.Context(), domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(domain.ScannedEvent) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadRecordsNamesAQueryFailure(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	if _, err := st.DB.ExecContext(t.Context(), "DROP TABLE event_log"); err != nil {
		t.Fatal(err)
	}
	err := st.ReadRecords(t.Context(), domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(domain.ScannedEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "query event log") {
		t.Fatalf("err = %v, want query event log", err)
	}
}

func TestReadEntityEventsIsScopedByTenantEntityAndInclusiveTimeBounds(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	at := func(minutes int) time.Time { return eventTime.Add(time.Duration(minutes) * time.Minute) }
	for id, spec := range map[string]struct {
		tenant, entity string
		minutes        int
	}{
		"before": {"tenant", "motor-1", -1}, "from": {"tenant", "motor-1", 0}, "inside": {"tenant", "motor-1", 5},
		"until": {"tenant", "motor-1", 10}, "after": {"tenant", "motor-1", 11},
		"other-entity": {"tenant", "motor-2", 5}, "other-tenant": {"other", "motor-1", 5},
	} {
		env := validEnvelope(id)
		env.TenantID, env.Entity.ID, env.EventTime = spec.tenant, spec.entity, at(spec.minutes)
		appendOne(t, st, env)
	}
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: at(0), Until: at(10), MaxRows: 10}
	if got := entityWindowIDs(t, st, window, 10); !slices.Equal(got, []string{"from", "inside", "until"}) {
		t.Fatalf("window = %v, want from, inside, until", got)
	}
	window.MaxRows = 2
	if got := entityWindowIDs(t, st, window, 10); !slices.Equal(got, []string{"from", "inside"}) {
		t.Fatalf("MaxRows 2 = %v, want the two earliest", got)
	}
	window.MaxRows = 10
	if got := entityWindowIDs(t, st, window, 1); !slices.Equal(got, []string{"from"}) {
		t.Fatalf("visit stopping after one = %v, want from", got)
	}
}

func TestReadEntityEventsBreaksEventTimeTiesByLogPosition(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	for _, id := range []string{"z-arrived-first", "a-arrived-second"} {
		appendOne(t, st, validEnvelope(id))
	}
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: eventTime, Until: eventTime, MaxRows: 10}
	want := []string{"z-arrived-first", "a-arrived-second"}
	if got := entityWindowIDs(t, st, window, 10); !slices.Equal(got, want) {
		t.Fatalf("tied event times = %v, want arrival order %v", got, want)
	}
}

func TestReadEntityEventsPropagatesTheVisitorFailureAndNamesQueryFailures(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: eventTime, Until: eventTime, MaxRows: 10}
	stopped := errors.New("visitor stopped")
	err := st.ReadEntityEvents(t.Context(), window, func(domain.EntityEvent) (bool, error) { return true, stopped })
	if !errors.Is(err, stopped) {
		t.Fatalf("err = %v, want the visitor error", err)
	}
	if _, err := st.DB.ExecContext(t.Context(), "DROP TABLE event_log"); err != nil {
		t.Fatal(err)
	}
	err = st.ReadEntityEvents(t.Context(), window, func(domain.EntityEvent) (bool, error) { return true, nil })
	if err == nil || !strings.Contains(err.Error(), "query evidence events") {
		t.Fatalf("err = %v, want query evidence events", err)
	}
}

func entityWindowIDs(t *testing.T, st Store, window domain.EntityWindow, stopAfter int) []string {
	t.Helper()
	var ids []string
	err := st.ReadEntityEvents(t.Context(), window, func(e domain.EntityEvent) (bool, error) {
		ids = append(ids, e.EventID)
		return len(ids) < stopAfter, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}
