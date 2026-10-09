package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestPipelineKeepsIngestingWhenCostReservationIsRejected(t *testing.T) {
	t.Parallel()
	ceiling := uint64(1)
	pipeline, db := openPipeline(t, thingSpec("native", "", withCostMicrounits(2)), runtime.PipelineConfig{GlobalCostCeiling: &ceiling})
	trace := writeTrace(t, levelEvent("station-a-001", "station-a", 15), levelEvent("station-b-001", "station-b", 15))

	report, err := pipeline.RunJSONL(t.Context(), trace)
	if err != nil {
		t.Fatalf("cost rejection stopped the pipeline: %v", err)
	}
	if report.EventsIngested != 2 || report.EventsProcessed != 2 || report.EpisodesAdmitted != 0 {
		t.Fatalf("report = %+v, want 2 events ingested and processed, no episode", report)
	}
	if events, episodes, skipped := countRows(t, db, "event_log"), countRows(t, db, "episodes"), scalar[int](t, db, "SELECT COUNT(*) FROM scheduler_items WHERE status = 'coalesced'"); events != 2 || episodes != 0 || skipped != 2 {
		t.Fatalf("events=%d episodes=%d coalesced items=%d; want 2, 0, 2", events, episodes, skipped)
	}
	reasons := scalar[string](t, db, "SELECT CAST(te.reasons_json AS TEXT) FROM trigger_evaluations te JOIN scheduler_items si ON si.trigger_id = te.trigger_id ORDER BY si.scheduler_item_id LIMIT 1")
	if !strings.Contains(reasons, "cost ceiling or kill switch rejected") {
		t.Fatalf("cost rejection reason = %q", reasons)
	}
}
