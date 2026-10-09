package telemetry

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/transport"
)

// Runtime holds process-local counters for the live pipeline. Counters are
// monotonic and have no tenant, entity, or event-id labels.
type Runtime = domain.Runtime

// PipelineReport is the small counter projection accepted from runtime.
type PipelineReport = domain.PipelineReport

// NewRuntime creates an operational counter set that traces through the
// process tracer provider.
func NewRuntime(now time.Time) *Runtime {
	return domain.NewRuntime(now, otel.Tracer(transport.InstrumentationName))
}

// MetricsHandler exposes the runtime's counters and latencies as Prometheus
// text without leaking tenant or event data.
func MetricsHandler(runtime *Runtime, gauges GaugeSource) http.Handler {
	return transport.MetricsHandler(runtime, gauges)
}

// GaugeSource reads point-in-time gauges, such as queue depths, when metrics
// are scraped.
type GaugeSource = transport.GaugeSource

// Configure installs the process tracer provider and returns it for deferred
// shutdown. The caller owns the provider lifecycle.
func Configure(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	return transport.Configure(ctx, serviceName, endpoint) //nolint:wrapcheck // The transport names the failed step.
}

// StartSpan starts an application span using the process tracer provider.
func StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return transport.StartSpan(ctx, name, options...)
}

// AddLinkFromW3C adds a causal link from a durable W3C trace context.
func AddLinkFromW3C(span trace.Span, traceparent, tracestate string) bool {
	return transport.AddLinkFromW3C(span, traceparent, tracestate)
}

// RecordError marks a span as failed without leaking request or evidence
// payloads into telemetry.
func RecordError(span trace.Span, err error) {
	transport.RecordError(span, err)
}
