package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const validTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func newRecordingProvider(t *testing.T, serviceName string) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	provider, err := NewTracerProvider(t.Context(), serviceName, "")
	if err != nil {
		t.Fatalf("NewTracerProvider: %v", err)
	}
	exporter := tracetest.NewInMemoryExporter()
	provider.RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.WithoutCancel(t.Context())) })
	return provider, exporter
}

func TestTracerProviderNamesTheServiceAndDefaultsToAgenticStream(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, serviceName, want string }{
		{"named", "svc", "svc"},
		{"unnamed", "", "agentic-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, exporter := newRecordingProvider(t, tc.serviceName)

			_, span := provider.Tracer("test").Start(t.Context(), "op")
			span.End()

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("exported spans = %d, want 1", len(spans))
			}
			var got string
			for _, attribute := range spans[0].Resource.Attributes() {
				if string(attribute.Key) == "service.name" {
					got = attribute.Value.AsString()
				}
			}
			if got != tc.want {
				t.Fatalf("service.name = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTracerProviderExportsSpansOverOTLPHTTPToTheTracesPath(t *testing.T) {
	t.Parallel()
	type request struct {
		path, contentType string
		body              []byte
	}
	received := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		select {
		case received <- request{r.URL.Path, r.Header.Get("Content-Type"), body}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	provider, err := NewTracerProvider(t.Context(), "test-runtime", server.URL)
	if err != nil {
		t.Fatalf("NewTracerProvider: %v", err)
	}

	_, span := provider.Tracer("test").Start(t.Context(), "exported-span")
	span.End()
	if err := provider.Shutdown(t.Context()); err != nil {
		t.Fatalf("flush spans on shutdown: %v", err)
	}

	select {
	case got := <-received:
		if got.path != "/v1/traces" || got.contentType != "application/x-protobuf" {
			t.Fatalf("exporter posted to %q as %q, want /v1/traces as application/x-protobuf", got.path, got.contentType)
		}
		for _, want := range []string{"test-runtime", "exported-span"} {
			if !strings.Contains(string(got.body), want) {
				t.Errorf("exported payload does not contain %q", want)
			}
		}
	default:
		t.Fatal("OTLP exporter sent no request before shutdown returned")
	}
}

func TestTracerProviderRejectsAnUnparsableEndpoint(t *testing.T) {
	t.Parallel()
	provider, err := NewTracerProvider(t.Context(), "svc", "http://[::1")
	if err == nil || !strings.Contains(err.Error(), "parse OTLP endpoint") {
		t.Fatalf("NewTracerProvider = %v, %v; want a parse OTLP endpoint failure", provider, err)
	}
}

func TestAddLinkFromW3CLinksOnlyAValidTraceContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		traceparent string
		tracestate  string
		wantLinked  bool
	}{
		{"valid", validTraceparent, "vendor=value", true},
		{"valid without tracestate", validTraceparent, "", true},
		{"empty", "", "", false},
		{"garbage", "garbage", "", false},
		{"all-zero trace id", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, exporter := newRecordingProvider(t, "svc")
			_, span := provider.Tracer("test").Start(t.Context(), "pipeline")

			linked := AddLinkFromW3C(span, tc.traceparent, tc.tracestate)
			span.End()

			if linked != tc.wantLinked {
				t.Fatalf("AddLinkFromW3C = %v, want %v", linked, tc.wantLinked)
			}
			links := exporter.GetSpans()[0].Links
			if !tc.wantLinked {
				if len(links) != 0 {
					t.Fatalf("an unlinked span has %d links", len(links))
				}
				return
			}
			if len(links) != 1 || links[0].SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" ||
				links[0].SpanContext.SpanID().String() != "00f067aa0ba902b7" || links[0].SpanContext.TraceState().String() != tc.tracestate {
				t.Fatalf("links = %+v, want one to the durable context with tracestate %q", links, tc.tracestate)
			}
		})
	}
	if AddLinkFromW3C(nil, validTraceparent, "") {
		t.Fatal("a nil span was linked")
	}
}

func TestRecordErrorMarksTheSpanFailedWithAFixedDescription(t *testing.T) {
	t.Parallel()
	provider, exporter := newRecordingProvider(t, "svc")
	tracer := provider.Tracer("test")

	_, untouched := tracer.Start(t.Context(), "no-error")
	RecordError(untouched, nil)
	RecordError(nil, errors.New("recorded on no span"))
	untouched.End()
	_, failing := tracer.Start(t.Context(), "failing")
	RecordError(failing, errors.New("boom"))
	failing.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want 2", len(spans))
	}
	if spans[0].Status.Code != codes.Unset || len(spans[0].Events) != 0 {
		t.Fatalf("a nil error changed the span: status %+v, events %v", spans[0].Status, spans[0].Events)
	}
	if spans[1].Status.Code != codes.Error || spans[1].Status.Description != "operation failed" {
		t.Fatalf("status = %+v, want Error \"operation failed\"", spans[1].Status)
	}
	if !slices.ContainsFunc(spans[1].Events, func(event sdktrace.Event) bool { return event.Name == "exception" }) {
		t.Fatalf("events = %v, want the recorded exception", spans[1].Events)
	}
}

//nolint:paralleltest // Configure replaces the process-wide tracer provider and propagator.
func TestConfigureInstallsTheProcessTracerProviderAndPropagator(t *testing.T) {
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	if provider, err := Configure(t.Context(), "svc", "http://[::1"); err == nil {
		t.Fatalf("Configure accepted an unparsable endpoint: %v", provider)
	}
	if otel.GetTracerProvider() != previousProvider {
		t.Fatal("a failed Configure replaced the process tracer provider")
	}

	provider, err := Configure(t.Context(), "svc", "")
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	exporter := tracetest.NewInMemoryExporter()
	provider.RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.WithoutCancel(t.Context())) })

	_, span := StartSpan(t.Context(), "op")
	span.End()

	if otel.GetTracerProvider() != provider {
		t.Fatal("Configure did not install the returned provider")
	}
	if spans := exporter.GetSpans(); len(spans) != 1 || spans[0].Name != "op" || spans[0].InstrumentationScope.Name != InstrumentationName {
		t.Fatalf("spans = %v, want one \"op\" span from the %s tracer", spans, InstrumentationName)
	}
	if fields := otel.GetTextMapPropagator().Fields(); !slices.Contains(fields, "traceparent") {
		t.Fatalf("propagator fields = %v, want W3C traceparent", fields)
	}
}
