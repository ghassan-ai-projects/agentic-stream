package eventlog_test

import (
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

func TestReadEntityWindowIsScopedToTheEntityOrderedByEventTimeAndStoppable(t *testing.T) {
	t.Parallel()
	log, db := newLog(t)
	var envelopes []contractsv1.Envelope
	for i, entity := range []string{"motor-1", "motor-2", "motor-1", "motor-1"} {
		event := envelope([]string{"evt-0", "evt-1", "evt-2", "evt-3"}[i], "default")
		event.PartitionKey, event.Entity.ID = entity, entity
		event.EventTime = eventTime.Add(time.Duration(i) * time.Minute)
		envelopes = append(envelopes, event)
	}
	if _, err := log.Append(t.Context(), "default", envelopes); err != nil {
		t.Fatal(err)
	}
	visit := func(window eventlog.EntityWindow, stopAfter int) []string {
		var ids []string
		err := eventlog.ReadEntityWindow(t.Context(), db, window, func(event eventlog.EntityEvent) (bool, error) {
			ids = append(ids, event.EventID)
			return len(ids) < stopAfter, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}

	window := eventlog.EntityWindow{TenantID: "default", EntityID: "motor-1", From: eventTime, Until: eventTime.Add(time.Hour), MaxRows: 10}
	if got := visit(window, 10); !slices.Equal(got, []string{"evt-0", "evt-2", "evt-3"}) {
		t.Fatalf("window = %v, want motor-1's events in event-time order", got)
	}
	if got := visit(window, 1); !slices.Equal(got, []string{"evt-0"}) {
		t.Fatalf("visit stopped after one = %v, want evt-0", got)
	}
	window.MaxRows = 2
	if got := visit(window, 10); !slices.Equal(got, []string{"evt-0", "evt-2"}) {
		t.Fatalf("MaxRows 2 = %v, want the two earliest", got)
	}
}
