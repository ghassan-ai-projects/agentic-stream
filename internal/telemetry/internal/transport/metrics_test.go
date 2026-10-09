package transport

import (
	"bufio"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/domain"
)

func TestMetricsHandlerServesEveryCounterAndLatencyAsUnlabeledPrometheusText(t *testing.T) {
	t.Parallel()
	runtime := domain.NewRuntime(time.Unix(1, 0), nil)
	runtime.ObservePipeline(domain.PipelineReport{EventsIngested: 2, CommandsDispatched: 1})
	runtime.ObserveStaleRejection()
	runtime.ObserveDuration(10 * time.Millisecond)
	response := httptest.NewRecorder()

	MetricsHandler(runtime, nil).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	if got := response.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("Content-Type = %q", got)
	}
	want := make(map[string]uint64)
	maps.Copy(want, runtime.Snapshot())
	maps.Copy(want, runtime.LatencySnapshot())
	got := parseMetrics(t, response.Body.String())
	if !maps.Equal(got, want) {
		t.Fatalf("served metrics differ from the runtime's views:\ngot  %v\nwant %v", got, want)
	}
	if got["agentic_stream_events_ingested_total"] != 2 || got["agentic_stream_dispatch_decision_p95_ns"] != 10_000_000 {
		t.Fatalf("served values do not reflect the observations: %v", got)
	}
	if strings.ContainsAny(response.Body.String(), "{}") || strings.Contains(response.Body.String(), "tenant") {
		t.Fatalf("metrics carry labels:\n%s", response.Body.String())
	}
}

func parseMetrics(t *testing.T, body string) map[string]uint64 {
	t.Helper()
	metrics := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "# TYPE ") {
			continue
		}
		name, text, ok := strings.Cut(scanner.Text(), " ")
		value, err := strconv.ParseUint(text, 10, 64)
		if !ok || err != nil {
			t.Fatalf("metric line %q is not \"name value\"", scanner.Text())
		}
		metrics[name] = value
	}
	return metrics
}

func TestMetricsAreSortedTypedAndIncludeScrapeTimeGauges(t *testing.T) {
	t.Parallel()
	runtime := domain.NewRuntime(time.Unix(1, 0), nil)
	gauges := func(context.Context) (map[string]float64, error) {
		return map[string]float64{"agentic_stream_ingest_lag_events": 3, "agentic_stream_database_bytes": 4096}, nil
	}
	response := httptest.NewRecorder()
	MetricsHandler(runtime, gauges).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, want := range []string{"# TYPE agentic_stream_events_ingested_total counter\n", "# TYPE agentic_stream_ingest_lag_events gauge\nagentic_stream_ingest_lag_events 3\n", "agentic_stream_database_bytes 4096\n"} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics lack %q:\n%s", want, body)
		}
	}
	var counters []string
	for _, line := range strings.Split(body, "\n") {
		if name, ok := strings.CutPrefix(line, "# TYPE "); ok && strings.HasSuffix(name, " counter") {
			counters = append(counters, name)
		}
	}
	if !slices.IsSorted(counters) {
		t.Fatalf("counters are not sorted: %v", counters)
	}
}

func TestUnavailableGaugesLeaveTheCountersServed(t *testing.T) {
	t.Parallel()
	failing := func(context.Context) (map[string]float64, error) { return nil, errors.New("database closed") }
	response := httptest.NewRecorder()
	MetricsHandler(domain.NewRuntime(time.Unix(1, 0), nil), failing).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	if !strings.Contains(response.Body.String(), "agentic_stream_events_ingested_total 0") {
		t.Fatalf("counters missing when gauges fail:\n%s", response.Body.String())
	}
}
