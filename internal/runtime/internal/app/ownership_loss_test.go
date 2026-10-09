package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

func TestAPipelineThatLostOwnershipIngestsNothing(t *testing.T) {
	t.Parallel()
	db := openDatabase(t)
	owner := claimOwnership(t, db, "epoch-lost")
	pipeline := newPipeline(t, db, thingSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-lost", Owner: owner})
	if err := owner.Release(t.Context(), "epoch-lost"); err != nil {
		t.Fatal(err)
	}
	sources := map[string]func() error{
		"normalized trace": func() error { _, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t)); return err },
		"simulator trace":  func() error { _, err := pipeline.RunSimulatorJSONL(t.Context(), highLevelTrace(t)); return err },
		"live socket": func() error {
			bounded, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			return pipeline.RunLiveSocket(bounded, filepath.Join(workerfake.SocketDir(t), "live.sock"))
		},
	}
	for name, run := range sources {
		if err := run(); !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
			t.Errorf("%s without ownership ended with %v, want %v", name, err, runtimecontrol.ErrRuntimeOwnerBusy)
		}
	}
	if events := countRows(t, db, "event_log"); events != 0 {
		t.Fatalf("%d events ingested after ownership was lost", events)
	}
}
