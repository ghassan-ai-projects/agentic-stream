package eventlog_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestReadEntityWindowIsScopedOrderedAndStoppable(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	appendEvents(t, db, start, []string{"motor-1", "motor-2", "motor-1", "motor-1"})

	window := eventlog.EntityWindow{TenantID: "default", EntityID: "motor-1", From: start, Until: start.Add(time.Hour), MaxRows: 10}
	if got := visitedIDs(t, db, window, 10); len(got) != 3 || got[0] != "evt-0" || got[1] != "evt-2" || got[2] != "evt-3" {
		t.Fatalf("window events = %v, want motor-1 events in event-time order", got)
	}
	if got := visitedIDs(t, db, window, 1); len(got) != 1 {
		t.Fatalf("visit stop returned %v, want exactly one event", got)
	}
	window.MaxRows = 2
	if got := visitedIDs(t, db, window, 10); len(got) != 2 {
		t.Fatalf("row limit returned %v, want two events", got)
	}
}

func appendEvents(t *testing.T, db *storage.DB, start time.Time, entities []string) {
	t.Helper()
	envelopes := make([]contractsv1.Envelope, 0, len(entities))
	for i, entity := range entities {
		envelopes = append(envelopes, contractsv1.Envelope{
			ID: "evt-" + string(rune('0'+i)), Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default",
			Source: "test", PartitionKey: entity, Entity: contractsv1.EntityRef{Type: "motor", ID: entity},
			EventTime: start.Add(time.Duration(i) * time.Minute), IngestedAt: start.Add(time.Hour),
			Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 40.0 + float64(i)},
		})
	}
	if _, err := eventlog.NewEventLog(db).Append(t.Context(), "default", envelopes); err != nil {
		t.Fatal(err)
	}
}

// visitedIDs reads the window, stopping after limit events.
func visitedIDs(t *testing.T, db *storage.DB, window eventlog.EntityWindow, limit int) []string {
	t.Helper()
	var ids []string
	err := eventlog.ReadEntityWindow(t.Context(), db, window, func(event eventlog.EntityEvent) (bool, error) {
		ids = append(ids, event.EventID)
		return len(ids) < limit, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}
