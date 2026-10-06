package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var traceparentPattern = regexp.MustCompile(`^(00|[0-9a-f]{2})-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// TraceContext is the W3C trace context carried across asynchronous runtime
// boundaries. A span link preserves the causal trace without pretending that
// a later worker or outcome is a child span of an already-finished operation.
type TraceContext struct {
	Traceparent string
	Tracestate  string
}

// SpanLink is the immutable trace context reference attached to an
// asynchronous stage.
type SpanLink struct {
	Traceparent string
	Tracestate  string
}

// ParseTraceContext validates W3C traceparent/tracestate values.
func ParseTraceContext(traceparent, tracestate string) (TraceContext, error) {
	if traceparent == "" && tracestate != "" {
		return TraceContext{}, fmt.Errorf("tracestate requires traceparent")
	}
	if traceparent == "" {
		return TraceContext{}, nil
	}
	if err := checkTraceparent(traceparent); err != nil {
		return TraceContext{}, err
	}
	if len(tracestate) > 512 || strings.ContainsAny(tracestate, "\r\n") {
		return TraceContext{}, fmt.Errorf("invalid tracestate")
	}
	return TraceContext{Traceparent: traceparent, Tracestate: tracestate}, nil
}

// checkTraceparent requires the W3C format with non-zero trace and span IDs
// and a trace ID that is not all ones.
func checkTraceparent(traceparent string) error {
	if !traceparentPattern.MatchString(traceparent) {
		return fmt.Errorf("invalid W3C traceparent")
	}
	parts := strings.Split(traceparent, "-")
	if strings.Trim(parts[1], "0") == "" || strings.Trim(parts[2], "0") == "" || parts[1] == strings.Repeat("f", 32) {
		return fmt.Errorf("traceparent contains an invalid zero or all-ones identifier")
	}
	return nil
}

// Link returns a span link for the context, or nil when tracing is absent.
func (c TraceContext) Link() *SpanLink {
	if c.Traceparent == "" {
		return nil
	}
	return &SpanLink{Traceparent: c.Traceparent, Tracestate: c.Tracestate}
}
