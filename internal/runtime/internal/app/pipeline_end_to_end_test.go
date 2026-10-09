package app_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestPipelineCompletesDecisionToSimulatedOutcome(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	report, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t))
	if err != nil {
		t.Fatal(err)
	}
	if report.EventsIngested != 1 || report.EpisodesAdmitted != 1 || report.EpisodesExecuted != 1 || report.IntentsEvaluated != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("report = %+v, want one event through one episode, intent and command", report)
	}
	if status := scalar[string](t, db, "SELECT status FROM commands"); status != "succeeded" {
		t.Fatalf("command status = %q, want succeeded", status)
	}
}

func TestPipelineIngestsASimulatorTraceThroughTheSameGovernedStages(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withSimulatorInput), runtime.PipelineConfig{})
	report, err := pipeline.RunSimulatorJSONL(t.Context(), writeSimulatorTrace(t, 15))
	if err != nil {
		t.Fatal(err)
	}
	if report.EventsIngested != 1 || report.EpisodesAdmitted != 1 || report.CommandsDispatched != 1 {
		t.Fatalf("report = %+v, want one simulator event through one episode and command", report)
	}
	if status := scalar[string](t, db, "SELECT status FROM commands"); status != "succeeded" {
		t.Fatalf("command status = %q, want succeeded", status)
	}
}

func TestPipelineNamesTheSourceThatFailedToIngest(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	sources := map[string]struct {
		run  func() error
		want string
	}{
		"normalized": {func() error { _, err := pipeline.RunJSONL(t.Context(), missing); return err }, "ingest live JSONL"},
		"simulator":  {func() error { _, err := pipeline.RunSimulatorJSONL(t.Context(), missing); return err }, "ingest simulator JSONL"},
	}
	for name, source := range sources {
		if err := source.run(); err == nil || !strings.Contains(err.Error(), source.want) || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s source error = %v, want %q wrapping a missing file", name, err, source.want)
		}
	}
	if events := countRows(t, db, "event_log"); events != 0 {
		t.Fatalf("%d events ingested from missing files", events)
	}
}
