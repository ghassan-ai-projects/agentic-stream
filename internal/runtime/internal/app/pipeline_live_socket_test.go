package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestFireRecentWatchesPaginatesPastFullPage(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "watch-pagination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	watch := newWatch(t, db)
	if _, err := watch.Dispatch(ctx, actionport.Command{
		CommandID:     "watch-pagination",
		TenantID:      "default",
		EffectorRoute: "install_watch_condition",
		Payload: map[string]any{
			"expression":        "features.level >= 1000",
			"target":            "target-final",
			"expires_at":        "2099-01-01T00:00:00Z",
			"situation_id":      "situation-watch-pagination",
			"situation_version": 1,
			"max_fires":         1,
		},
	}); err != nil {
		t.Fatalf("install pagination watch: %v", err)
	}

	envelopes := make([]contractsv1.Envelope, watchReadBatchSize+1)
	for i := range envelopes {
		entityID := "other-" + fmt.Sprint(i)
		if i == len(envelopes)-1 {
			entityID = "target-final"
		}
		envelopes[i] = contractsv1.Envelope{
			ID:             fmt.Sprintf("evt-watch-pagination-%d", i),
			Type:           "sensor.temperature",
			SchemaVersion:  "1.0",
			TenantID:       "default",
			Source:         "test",
			PartitionKey:   entityID,
			Entity:         contractsv1.EntityRef{Type: "motor", ID: entityID},
			EventTime:      time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			IngestedAt:     time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			Classification: contractsv1.ClassificationInternal,
			Data:           map[string]any{"level": float64(i)},
		}
	}
	log := eventlog.NewEventLog(db)
	if positions, err := log.Append(ctx, "default", envelopes); err != nil {
		t.Fatalf("append pagination events: %v", err)
	} else if len(positions) != len(envelopes) {
		t.Fatalf("appended positions = %d, want %d", len(positions), len(envelopes))
	}

	pipeline := &Pipeline{log: log, watch: watch, tenantID: "default"}
	spanCtx, span := noop.NewTracerProvider().Tracer("runtime-test").Start(ctx, "watch-pagination")
	defer span.End()
	if err := pipeline.fireRecentWatches(spanCtx, 0, span); err != nil {
		t.Fatalf("fire paginated watches: %v", err)
	}

	var fires, remaining int
	var status string
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM watch_fires WHERE watch_id = ?", "watch-pagination").Scan(&fires); err != nil {
		t.Fatalf("count pagination watch fires: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT status, remaining_fires FROM watch_conditions WHERE watch_id = ?", "watch-pagination").Scan(&status, &remaining); err != nil {
		t.Fatalf("load pagination watch: %v", err)
	}
	if fires != 1 || status != "disabled" || remaining != 0 {
		t.Fatalf("pagination watch state: fires=%d status=%q remaining=%d; want 1, disabled, 0", fires, status, remaining)
	}
}
