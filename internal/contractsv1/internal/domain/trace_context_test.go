package domain_test

import (
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

const validTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestParseTraceContextKeepsAValidContextAndLinksToIt(t *testing.T) {
	t.Parallel()
	ctx, err := domain.ParseTraceContext(validTraceparent, "vendor=value")
	if err != nil {
		t.Fatalf("parse trace context: %v", err)
	}
	link := ctx.Link()
	if link == nil || link.Traceparent != validTraceparent || link.Tracestate != "vendor=value" {
		t.Fatalf("span link = %#v, want the parsed traceparent and tracestate", link)
	}
}

func TestParseTraceContextAcceptsNoTracing(t *testing.T) {
	t.Parallel()
	ctx, err := domain.ParseTraceContext("", "")
	if err != nil || ctx != (domain.TraceContext{}) || ctx.Link() != nil {
		t.Fatalf("ParseTraceContext(empty) = %+v, %v, link %v; want the zero context and no link", ctx, err, ctx.Link())
	}
}

func TestParseTraceContextRefusesInvalidContexts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, parent, state, want string
	}{
		{"tracestate without traceparent", "", "vendor=value", "tracestate requires traceparent"},
		{"zero trace id", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", "", "zero or all-ones"},
		{"zero span id", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", "", "zero or all-ones"},
		{"all-ones trace id", "00-ffffffffffffffffffffffffffffffff-00f067aa0ba902b7-01", "", "zero or all-ones"},
		{"upper case hex", strings.ToUpper(validTraceparent), "", "invalid W3C traceparent"},
		{"missing flags", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7", "", "invalid W3C traceparent"},
		{"short trace id", "00-4bf92f3577b34da6a3ce929d0e0e47-00f067aa0ba902b7-01", "", "invalid W3C traceparent"},
		{"non hex version", "zz-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "", "invalid W3C traceparent"},
		{"garbage", "garbage", "", "invalid W3C traceparent"},
		{"tracestate over 512 bytes", validTraceparent, strings.Repeat("a", 513), "invalid tracestate"},
		{"tracestate with a line break", validTraceparent, "a=b\nc=d", "invalid tracestate"},
		{"tracestate with a carriage return", validTraceparent, "a=b\rc=d", "invalid tracestate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, err := domain.ParseTraceContext(tt.parent, tt.state)
			if err == nil || !strings.Contains(err.Error(), tt.want) || ctx != (domain.TraceContext{}) {
				t.Fatalf("ParseTraceContext = %+v, %v; want the zero context and an error containing %q", ctx, err, tt.want)
			}
		})
	}
}

func TestParseTraceContextAcceptsATracestateOfExactly512Bytes(t *testing.T) {
	t.Parallel()
	if _, err := domain.ParseTraceContext(validTraceparent, strings.Repeat("a", 512)); err != nil {
		t.Fatalf("tracestate at the limit refused: %v", err)
	}
}
