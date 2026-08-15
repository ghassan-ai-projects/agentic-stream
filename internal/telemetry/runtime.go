// Package telemetry contains the runtime's low-cardinality operational
// measurements. It is intentionally provider-neutral; deployments can bridge
// the snapshot to OpenTelemetry or Prometheus without changing stream logic.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Runtime holds process-local counters for the live pipeline. Counters are
// monotonic and have no tenant, entity, or event-id labels.
type Runtime struct {
	started            time.Time
	tracer             trace.Tracer
	eventsIngested     atomic.Uint64
	eventsProcessed    atomic.Uint64
	episodesAdmitted   atomic.Uint64
	episodesExecuted   atomic.Uint64
	intentsEvaluated   atomic.Uint64
	commandsDispatched atomic.Uint64
	streamFailures     atomic.Uint64
	// P8: the freshness/latency surface — stale-decision rejections and the
	// dispatch→decision duration histogram (p95/p99), exported via /metrics
	// so the freshness SLO is one honest number.
	staleRejections atomic.Uint64
	durationsMu     sync.Mutex
	durations       []time.Duration
}

// NewRuntime creates an operational counter set.
func NewRuntime(now time.Time) *Runtime {
	return NewRuntimeWithTracer(now, otel.Tracer(instrumentationName))
}

// NewRuntimeWithTracer creates an operational counter set with an explicit
// tracer, which keeps tests and embedded runtimes independent of global setup.
func NewRuntimeWithTracer(now time.Time, tracer trace.Tracer) *Runtime {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	return &Runtime{started: now.UTC(), tracer: tracer}
}

// StartSpan starts a runtime span when this telemetry runtime is configured.
func (r *Runtime) StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	if r == nil || r.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return r.tracer.Start(ctx, name, options...)
}

// PipelineReport is the small counter projection accepted from runtime.
type PipelineReport struct {
	EventsIngested     int
	EventsProcessed    int
	EpisodesAdmitted   int
	EpisodesExecuted   int
	IntentsEvaluated   int
	CommandsDispatched int
}

// ObservePipeline records one completed pipeline batch.
func (r *Runtime) ObservePipeline(report PipelineReport) {
	if r == nil {
		return
	}
	add := func(counter *atomic.Uint64, value int) {
		if value > 0 {
			counter.Add(uint64(value))
		}
	}
	add(&r.eventsIngested, report.EventsIngested)
	add(&r.eventsProcessed, report.EventsProcessed)
	add(&r.episodesAdmitted, report.EpisodesAdmitted)
	add(&r.episodesExecuted, report.EpisodesExecuted)
	add(&r.intentsEvaluated, report.IntentsEvaluated)
	add(&r.commandsDispatched, report.CommandsDispatched)
}

// ObserveFailure increments the bounded pipeline failure counter.
func (r *Runtime) ObserveFailure() {
	if r != nil {
		r.streamFailures.Add(1)
	}
}

// ObserveStaleRejection increments the stale-decision rejection counter.
func (r *Runtime) ObserveStaleRejection() {
	if r != nil {
		r.staleRejections.Add(1)
	}
}

// ObserveDuration records one dispatch→decision duration for the histogram.
func (r *Runtime) ObserveDuration(duration time.Duration) {
	if r == nil {
		return
	}
	r.durationsMu.Lock()
	defer r.durationsMu.Unlock()
	r.durations = append(r.durations, duration)
}

// Percentile returns the p-th percentile of the recorded durations (0-100),
// or 0 when no durations were recorded.
func (r *Runtime) Percentile(p float64) time.Duration {
	if r == nil {
		return 0
	}
	r.durationsMu.Lock()
	defer r.durationsMu.Unlock()
	if len(r.durations) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), r.durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int((p / 100) * float64(len(sorted)-1))
	return sorted[index]
}

// Snapshot returns a stable counter view.
func (r *Runtime) Snapshot() map[string]uint64 {
	if r == nil {
		return nil
	}
	return map[string]uint64{
		"agentic_stream_events_ingested_total":     r.eventsIngested.Load(),
		"agentic_stream_events_processed_total":    r.eventsProcessed.Load(),
		"agentic_stream_episodes_admitted_total":   r.episodesAdmitted.Load(),
		"agentic_stream_episodes_executed_total":   r.episodesExecuted.Load(),
		"agentic_stream_intents_evaluated_total":   r.intentsEvaluated.Load(),
		"agentic_stream_commands_dispatched_total": r.commandsDispatched.Load(),
		"agentic_stream_pipeline_failures_total":   r.streamFailures.Load(),
		"agentic_stream_stale_rejections_total":    r.staleRejections.Load(),
	}
}

// LatencySnapshot returns the p50/p95/p99 dispatch→decision latencies in
// nanoseconds (0 when no durations were recorded yet).
func (r *Runtime) LatencySnapshot() map[string]uint64 {
	return map[string]uint64{
		"agentic_stream_dispatch_decision_p50_ns": uint64(r.Percentile(50)),
		"agentic_stream_dispatch_decision_p95_ns": uint64(r.Percentile(95)),
		"agentic_stream_dispatch_decision_p99_ns": uint64(r.Percentile(99)),
	}
}

// Handler exposes low-cardinality Prometheus text without leaking tenant or
// event data. It is safe to mount behind the same local HTTP listener.
func (r *Runtime) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for name, value := range r.Snapshot() {
			_, _ = fmt.Fprintf(w, "%s %d\n", name, value)
		}
		for name, value := range r.LatencySnapshot() {
			_, _ = fmt.Fprintf(w, "%s %d\n", name, value)
		}
	})
}
