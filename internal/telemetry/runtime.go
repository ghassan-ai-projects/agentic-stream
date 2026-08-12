// Package telemetry contains the runtime's low-cardinality operational
// measurements. It is intentionally provider-neutral; deployments can bridge
// the snapshot to OpenTelemetry or Prometheus without changing stream logic.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
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
	})
}
