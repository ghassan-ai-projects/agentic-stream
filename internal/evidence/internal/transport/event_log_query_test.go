package transport

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func sensorEvent(id, entity string, at time.Time) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default", Source: "test", PartitionKey: entity,
		Entity: contractsv1.EntityRef{Type: "motor", ID: entity}, EventTime: at, IngestedAt: at,
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 42.0},
	}
}

func seededEventLog(t *testing.T) *storage.DB {
	t.Helper()
	db := openLedgerDB(t)
	events := []contractsv1.Envelope{
		sensorEvent("evt-1", "motor-1", fixedNow),
		sensorEvent("evt-2", "motor-1", fixedNow.Add(time.Second)),
		sensorEvent("evt-3", "motor-1", fixedNow.Add(time.Hour)),
		sensorEvent("evt-other", "motor-2", fixedNow),
	}
	if _, err := eventlog.NewEventLog(db).Append(t.Context(), "default", events); err != nil {
		t.Fatalf("append events: %v", err)
	}
	return db
}

func eventIDs(result domain.QueryResult) []string {
	var ids []string
	for _, part := range strings.Split(string(result.JSON), `"event_id":"`)[1:] {
		ids = append(ids, part[:strings.Index(part, `"`)])
	}
	return ids
}

func TestEventLogQueryReturnsTheExactScopedBytes(t *testing.T) {
	t.Parallel()
	query := EventLogQuery(seededEventLog(t))
	call := domain.Call{TenantID: "default", EntityID: "motor-1", From: fixedNow, Until: fixedNow, MaxRows: 1}
	result, err := query(t.Context(), call)
	const want = `{"rows":[{"data":{"celsius":42},"event_id":"evt-1","event_time":"2026-08-12T12:00:00.000000000Z","event_type":"sensor.temperature"}]}`
	if err != nil || result.RowCount != 1 || string(result.JSON) != want {
		t.Fatalf("result = %s (rows %d), err %v\nwant %s", result.JSON, result.RowCount, err, want)
	}
}

func TestEventLogQueryReadsOnlyWhatTheCallScopeAllows(t *testing.T) {
	t.Parallel()
	query := EventLogQuery(seededEventLog(t))
	tests := []struct {
		name    string
		mutate  func(*domain.Call)
		wantIDs []string
	}{
		{"the granted window", func(*domain.Call) {}, []string{"evt-1", "evt-2"}},
		{"another entity", func(c *domain.Call) { c.EntityID = "motor-2" }, []string{"evt-other"}},
		{"another tenant", func(c *domain.Call) { c.TenantID = "other" }, nil},
		{"an unknown entity", func(c *domain.Call) { c.EntityID = "motor-9" }, nil},
		{"the row budget", func(c *domain.Call) { c.MaxRows = 1 }, []string{"evt-1"}},
		{"a window before the events", func(c *domain.Call) { c.From, c.Until = fixedNow.Add(-time.Hour), fixedNow.Add(-time.Second) }, nil},
		{"a later window", func(c *domain.Call) { c.From, c.Until = fixedNow.Add(30*time.Minute), fixedNow.Add(2*time.Hour) }, []string{"evt-3"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			call := domain.Call{TenantID: "default", EntityID: "motor-1", From: fixedNow, Until: fixedNow.Add(time.Minute), MaxRows: 10}
			test.mutate(&call)
			result, err := query(t.Context(), call)
			if err != nil || !slices.Equal(eventIDs(result), test.wantIDs) || result.RowCount != uint64(len(test.wantIDs)) {
				t.Fatalf("events = %v (rows %d), err %v, want %v", eventIDs(result), result.RowCount, err, test.wantIDs)
			}
			if test.wantIDs == nil && string(result.JSON) != `{"rows":[]}` {
				t.Fatalf("result = %s, want an empty rows array", result.JSON)
			}
		})
	}
}

func TestEventLogRowDecodingRefusesPayloadsThatAreNotJSON(t *testing.T) {
	t.Parallel()
	if _, err := eventRow(eventlog.EntityEvent{Payload: []byte("bad")}); err == nil {
		t.Fatal("an unreadable payload was decoded")
	}
}

func TestEventLogQueryReportsStorageFailureWithContext(t *testing.T) {
	t.Parallel()
	db := seededEventLog(t)
	query := EventLogQuery(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := query(t.Context(), domain.Call{TenantID: "default", EntityID: "motor-1", From: fixedNow, Until: fixedNow, MaxRows: 1})
	if err == nil || !strings.Contains(err.Error(), "read evidence events") {
		t.Fatalf("error = %v, want a wrapped event read failure", err)
	}
}
