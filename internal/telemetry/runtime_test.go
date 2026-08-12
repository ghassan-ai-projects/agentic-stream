package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestDurableW3CContextBecomesOpenTelemetryLink(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	_, span := provider.Tracer("test").Start(context.Background(), "pipeline")
	if !telemetry.AddLinkFromW3C(span, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "vendor=value") {
		t.Fatal("expected valid W3C link")
	}
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 || len(spans[0].Links) != 1 {
		t.Fatalf("exported spans=%d links=%d", len(spans), len(spans[0].Links))
	}
	if got := spans[0].Links[0].SpanContext.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("linked trace id=%s", got)
	}
}

func TestOTLPHTTPProviderExportsSpans(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("path=%s", r.URL.Path)
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousProvider := otel.GetTracerProvider()
	defer otel.SetTracerProvider(previousProvider)
	provider, err := telemetry.Configure(context.Background(), "test-runtime", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, span := provider.Tracer("test").Start(context.Background(), "exported")
	span.End()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := provider.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requests:
	case <-shutdownCtx.Done():
		t.Fatal("OTLP exporter did not send a request")
	}
}
