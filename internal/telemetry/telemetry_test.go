package telemetry_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func TestMetricsHandlerServesTheRuntimeCountersWithoutLabels(t *testing.T) {
	t.Parallel()
	runtime := telemetry.NewRuntime(time.Unix(1, 0))
	runtime.ObservePipeline(telemetry.PipelineReport{EventsIngested: 2, CommandsDispatched: 1})
	runtime.ObserveFailure()
	response := httptest.NewRecorder()

	telemetry.MetricsHandler(runtime).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	lines := strings.Split(response.Body.String(), "\n")
	for _, want := range []string{
		"agentic_stream_events_ingested_total 2",
		"agentic_stream_commands_dispatched_total 1",
		"agentic_stream_pipeline_failures_total 1",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("metrics lack the line %q:\n%s", want, response.Body.String())
		}
	}
	if strings.ContainsAny(response.Body.String(), "{}") || strings.Contains(response.Body.String(), "tenant") {
		t.Fatalf("metrics carry labels:\n%s", response.Body.String())
	}
}

//nolint:paralleltest // Configure replaces the process-wide tracer provider and propagator.
func TestSpansStartedThroughTheFacadeReachTheConfiguredProvider(t *testing.T) {
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	provider, err := telemetry.Configure(t.Context(), "facade-test", "")
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	exporter := tracetest.NewInMemoryExporter()
	provider.RegisterSpanProcessor(trace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	ctx, pipeline := telemetry.StartSpan(t.Context(), "pipeline")
	linked := telemetry.AddLinkFromW3C(pipeline, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "")
	telemetry.RecordError(pipeline, errors.New("boom"))
	pipeline.End()
	_, runtimeSpan := telemetry.NewRuntime(time.Unix(1, 0)).StartSpan(ctx, "runtime-step")
	runtimeSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 || spans[0].Name != "pipeline" || spans[1].Name != "runtime-step" {
		t.Fatalf("exported spans = %v, want pipeline then runtime-step", spans)
	}
	if !linked || len(spans[0].Links) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("pipeline span: linked=%v links=%d status=%+v; want one link and an error status", linked, len(spans[0].Links), spans[0].Status)
	}
	if spans[1].Parent.SpanID() != spans[0].SpanContext.SpanID() {
		t.Fatal("the runtime span is not a child of the span in its context")
	}
}
