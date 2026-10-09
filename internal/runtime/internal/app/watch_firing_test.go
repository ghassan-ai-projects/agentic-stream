package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestWatchFiringReadsEveryPageOfRecentEvents(t *testing.T) {
	t.Parallel()
	const pageSize, events = 3, 3*2 + 1
	db := storagetest.OpenTemp(t)
	watch := NewWatch(t, db)
	installWatch(t, watch, "watch-pagination", "target-final")
	log := eventlog.NewEventLog(db)
	appendLevelEvents(t, log, events, "target-final")

	pipeline := &Pipeline{log: log, watch: watch, tenantID: "default", watchPageSize: pageSize}
	span := noopSpan(t)
	if err := pipeline.fireRecentWatches(t.Context(), 0, span); err != nil {
		t.Fatalf("fire paginated watches: %v", err)
	}

	fires := scalarInt(t, db, "SELECT COUNT(*) FROM watch_fires WHERE watch_id = 'watch-pagination'")
	status, remaining := watchState(t, db, "watch-pagination")
	if fires != 1 || status != "disabled" || remaining != 0 {
		t.Fatalf("watch fired %d times, status %q, remaining %d; want 1 fire, disabled, 0 (the target is on the last page)", fires, status, remaining)
	}
}

func TestWatchFiringOnlyReadsEventsAfterTheBatchStartPosition(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	watch := NewWatch(t, db)
	installWatch(t, watch, "watch-late", "target-final")
	log := eventlog.NewEventLog(db)
	appendLevelEvents(t, log, 4, "target-final")

	current, err := log.CurrentPosition(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := &Pipeline{log: log, watch: watch, tenantID: "default"}
	if err := pipeline.fireRecentWatches(t.Context(), current, noopSpan(t)); err != nil {
		t.Fatal(err)
	}
	if status, remaining := watchState(t, db, "watch-late"); status != "active" || remaining != 1 {
		t.Fatalf("a watch fired on events from before the batch: status %q, remaining %d", status, remaining)
	}
}

func installWatch(t *testing.T, service interface {
	Dispatch(context.Context, actionport.Command) (actionport.Effect, error)
}, id, target string) {
	t.Helper()
	_, err := service.Dispatch(t.Context(), actionport.Command{
		CommandID: id, TenantID: "default", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression": "features.level >= 1", "target": target, "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "situation-" + id, "situation_version": 1, "max_fires": 1,
		},
	})
	if err != nil {
		t.Fatalf("install watch %s: %v", id, err)
	}
}

func appendLevelEvents(t *testing.T, log *eventlog.EventLog, count int, finalEntity string) {
	t.Helper()
	envelopes := make([]contractsv1.Envelope, count)
	for i := range envelopes {
		entityID := fmt.Sprintf("other-%d", i)
		if i == count-1 {
			entityID = finalEntity
		}
		at := time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)
		envelopes[i] = contractsv1.Envelope{
			ID: fmt.Sprintf("evt-watch-%d", i), Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default", Source: "test",
			PartitionKey: entityID, Entity: contractsv1.EntityRef{Type: "motor", ID: entityID}, EventTime: at, IngestedAt: at,
			Classification: contractsv1.ClassificationInternal, Data: map[string]any{"level": float64(i)},
		}
	}
	if positions, err := log.Append(t.Context(), "default", envelopes); err != nil || len(positions) != count {
		t.Fatalf("append %d events: %d positions, %v", count, len(positions), err)
	}
}

func watchState(t *testing.T, db *storage.DB, watchID string) (string, int) {
	t.Helper()
	var status string
	var remaining int
	if err := db.QueryRowContext(t.Context(), "SELECT status, remaining_fires FROM watch_conditions WHERE watch_id = ?", watchID).Scan(&status, &remaining); err != nil {
		t.Fatalf("load watch %s: %v", watchID, err)
	}
	return status, remaining
}

func scalarInt(t *testing.T, db *storage.DB, query string) int {
	t.Helper()
	var value int
	if err := db.QueryRowContext(t.Context(), query).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

func TestWatchMaintenanceFailureStopsLaterBatches(t *testing.T) {
	t.Parallel()
	pipeline := &Pipeline{watch: NewWatch(t, storagetest.OpenTemp(t))}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	maintenanceErr := pipeline.expireMaintainedWatches(canceled)
	if !errors.Is(maintenanceErr, context.Canceled) {
		t.Fatalf("maintenance error = %v, want the cancellation", maintenanceErr)
	}
	if !errors.Is(pipeline.watchFailure(), context.Canceled) {
		t.Fatalf("recorded failure = %v", pipeline.watchFailure())
	}
	err := pipeline.advanceBatch(t.Context(), &PipelineReport{}, 0, noopSpan(t))
	if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "watch maintenance failed") {
		t.Fatalf("next batch error = %v, want the recorded maintenance failure", err)
	}
}

func noopSpan(t *testing.T) trace.Span {
	t.Helper()
	_, span := noop.NewTracerProvider().Tracer("runtime-test").Start(t.Context(), "watch")
	t.Cleanup(func() { span.End() })
	return span
}
