package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestStartMaintainsWatchesWithoutNewSensorInput(t *testing.T) {
	t.Parallel()
	db := openDatabase(t)
	if _, err := app.NewWatch(t, db).Dispatch(t.Context(), installWatchCommand()); err != nil {
		t.Fatal(err)
	}
	afterExpiry := sources.NewVirtual(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	pipeline := newPipeline(t, db, thingSpec("native", "active"), runtime.PipelineConfig{Clock: afterExpiry, MaintenanceInterval: time.Millisecond})
	if status := scalar[string](t, db, "SELECT status FROM watch_conditions WHERE watch_id = 'cmd-watch'"); status != "active" {
		t.Fatalf("watch status before maintenance = %q", status)
	}

	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, "maintenance to expire the watch", func() bool {
		return scalar[string](t, db, "SELECT status FROM watch_conditions WHERE watch_id = 'cmd-watch'") == "expired"
	})
	if err := pipeline.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceStartsOnceAndCanBeRestartedAfterClose(t *testing.T) {
	t.Parallel()
	pipeline, _ := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(t.Context()); err == nil {
		t.Fatal("a running pipeline started its maintenance twice")
	}
	for range 2 {
		if err := pipeline.Close(); err != nil {
			t.Fatalf("close must be idempotent: %v", err)
		}
	}
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatalf("restart after close: %v", err)
	}
}
