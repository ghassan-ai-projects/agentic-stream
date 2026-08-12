package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func TestRuntimeCountersAndMetricsAreLowCardinality(t *testing.T) {
	runtime := telemetry.NewRuntime(time.Unix(1, 0))
	runtime.ObservePipeline(telemetry.PipelineReport{EventsIngested: 2, EventsProcessed: 3, EpisodesAdmitted: 1, EpisodesExecuted: 1, IntentsEvaluated: 1, CommandsDispatched: 1})
	runtime.ObserveFailure()
	if got := runtime.Snapshot()["agentic_stream_events_ingested_total"]; got != 2 {
		t.Fatalf("events=%d", got)
	}
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	if !strings.Contains(response.Body.String(), "agentic_stream_pipeline_failures_total 1") || strings.Contains(response.Body.String(), "tenant") {
		t.Fatalf("metrics=%s", response.Body.String())
	}
}
