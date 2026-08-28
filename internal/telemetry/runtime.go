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
	// ISSUE-061: stale episodes recovered by re-binding to the live situation
	// version instead of being abandoned — the recovery counter, so a dense
	// trace's dispatch churn is visible, not silent.
	staleRebinds atomic.Uint64
	// ISSUE-061: re-binds that failed because the live snapshot did not
	// validate (DB corruption — the engine validates at publish). Counted
	// separately from stale_rejections so corruption is distinguishable from
	// benign churn in /metrics.
	rebindFailures        atomic.Uint64
	deviceFrameErrors     atomic.Uint64
	deviceReconnects      atomic.Uint64
	actionUnknownOutcomes atomic.Uint64
	verificationPending   atomic.Uint64
	verificationFailures  atomic.Uint64
	leaseExpiries         atomic.Uint64
	safeStateEntries      atomic.Uint64
	durationsMu           sync.Mutex
	durations             []time.Duration
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

// ObserveStaleRebind increments the stale-episode re-bind recovery counter.
func (r *Runtime) ObserveStaleRebind() {
	if r != nil {
		r.staleRebinds.Add(1)
	}
}

// ObserveRebindFailure increments the failed re-bind counter — a live snapshot
// that did not validate (corruption), distinct from a benign stale rejection.
func (r *Runtime) ObserveRebindFailure() {
	if r != nil {
		r.rebindFailures.Add(1)
	}
}

// ObserveDeviceFrameError increments the device-protocol decode/validation
// error counter.
func (r *Runtime) ObserveDeviceFrameError() {
	if r != nil {
		r.deviceFrameErrors.Add(1)
	}
}

// ObserveDeviceReconnect increments the gateway reconnect counter.
func (r *Runtime) ObserveDeviceReconnect() {
	if r != nil {
		r.deviceReconnects.Add(1)
	}
}

// ObserveActionUnknownOutcome increments the ambiguous-action counter.
func (r *Runtime) ObserveActionUnknownOutcome() {
	if r != nil {
		r.actionUnknownOutcomes.Add(1)
	}
}

// ObserveVerificationPending increments the transport-accepted, independently
// unverified action counter.
func (r *Runtime) ObserveVerificationPending() {
	if r != nil {
		r.verificationPending.Add(1)
	}
}

// ObserveVerificationFailure increments the independent-feedback failure
// counter.
func (r *Runtime) ObserveVerificationFailure() {
	if r != nil {
		r.verificationFailures.Add(1)
	}
}

// ObserveLeaseExpiry increments the action lease-expiry counter.
func (r *Runtime) ObserveLeaseExpiry() {
	if r != nil {
		r.leaseExpiries.Add(1)
	}
}

// ObserveSafeStateEntry records a transition into the device-reported safe
// state.
func (r *Runtime) ObserveSafeStateEntry() {
	if r != nil {
		r.safeStateEntries.Add(1)
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
		"agentic_stream_events_ingested_total":         r.eventsIngested.Load(),
		"agentic_stream_events_processed_total":        r.eventsProcessed.Load(),
		"agentic_stream_episodes_admitted_total":       r.episodesAdmitted.Load(),
		"agentic_stream_episodes_executed_total":       r.episodesExecuted.Load(),
		"agentic_stream_intents_evaluated_total":       r.intentsEvaluated.Load(),
		"agentic_stream_commands_dispatched_total":     r.commandsDispatched.Load(),
		"agentic_stream_pipeline_failures_total":       r.streamFailures.Load(),
		"agentic_stream_stale_rejections_total":        r.staleRejections.Load(),
		"agentic_stream_stale_rebinds_total":           r.staleRebinds.Load(),
		"agentic_stream_rebind_failures_total":         r.rebindFailures.Load(),
		"agentic_stream_device_frame_errors_total":     r.deviceFrameErrors.Load(),
		"agentic_stream_device_reconnects_total":       r.deviceReconnects.Load(),
		"agentic_stream_action_unknown_outcomes_total": r.actionUnknownOutcomes.Load(),
		"agentic_stream_verification_pending_total":    r.verificationPending.Load(),
		"agentic_stream_verification_failures_total":   r.verificationFailures.Load(),
		"agentic_stream_lease_expiries_total":          r.leaseExpiries.Load(),
		"agentic_stream_safe_state_entries_total":      r.safeStateEntries.Load(),
	}
}

// latencyNanos converts a recorded duration to uint64 nanoseconds, clamping
// negative values to zero so an anomalous clock cannot wrap the metric.
func latencyNanos(d time.Duration) uint64 {
	if d < 0 {
		return 0
	}
	return uint64(d)
}

// LatencySnapshot returns the p50/p95/p99 dispatch→decision latencies in
// nanoseconds (0 when no durations were recorded yet).
func (r *Runtime) LatencySnapshot() map[string]uint64 {
	return map[string]uint64{
		"agentic_stream_dispatch_decision_p50_ns": latencyNanos(r.Percentile(50)),
		"agentic_stream_dispatch_decision_p95_ns": latencyNanos(r.Percentile(95)),
		"agentic_stream_dispatch_decision_p99_ns": latencyNanos(r.Percentile(99)),
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
