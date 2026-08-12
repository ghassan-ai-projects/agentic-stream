package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/ghassan-ai-projects/agentic-stream"

// NewTracerProvider creates an OpenTelemetry tracer provider. When endpoint
// is empty, spans are recorded by the SDK but no exporter is configured. When
// endpoint is set, spans are exported using OTLP/HTTP with bounded batching.
func NewTracerProvider(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	if serviceName == "" {
		serviceName = "agentic-stream"
	}
	resource, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource)}
	if endpoint != "" {
		exporter, exportErr := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpoint),
			otlptracehttp.WithTimeout(5*time.Second),
		)
		if exportErr != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", exportErr)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	return sdktrace.NewTracerProvider(options...), nil
}

// Configure installs the process tracer provider and returns it for deferred
// shutdown. The caller owns the provider lifecycle.
func Configure(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	provider, err := NewTracerProvider(ctx, serviceName, endpoint)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider, nil
}

// StartSpan starts an application span using the process tracer provider.
func StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(instrumentationName).Start(ctx, name, options...)
}

// AddLinkFromW3C adds a causal link from a durable W3C trace context. Links
// are used for asynchronous work so a new runtime span is not falsely modeled
// as a child of an already-completed source operation.
func AddLinkFromW3C(span trace.Span, traceparent, tracestate string) bool {
	if span == nil || traceparent == "" {
		return false
	}
	carrier := propagation.MapCarrier{
		"traceparent": traceparent,
		"tracestate":  tracestate,
	}
	linked := propagation.TraceContext{}.Extract(context.Background(), carrier)
	spanContext := trace.SpanContextFromContext(linked)
	if !spanContext.IsValid() {
		return false
	}
	span.AddLink(trace.Link{SpanContext: spanContext})
	return true
}

// RecordError marks a span as failed without leaking request or evidence
// payloads into telemetry.
func RecordError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, "operation failed")
}
