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
	rebindFailures         atomic.Uint64
	deviceFrameErrors      atomic.Uint64
	deviceReconnects       atomic.Uint64
	actionUnknownOutcomes  atomic.Uint64
	verificationPending    atomic.Uint64
	verificationFailures   atomic.Uint64
	leaseExpiries          atomic.Uint64
	safeStateEntries       atomic.Uint64
	reconciliationBarriers atomic.Uint64
	targetClaimRejections  atomic.Uint64
	safeStopRequests       atomic.Uint64
	safeStopFailures       atomic.Uint64
	safeStopCompletions    atomic.Uint64
	liveLinesIngested      atomic.Uint64
	liveLinesRejected      atomic.Uint64
	durationsMu            sync.Mutex
	durations              []time.Duration
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
	snapshot := make(map[string]uint64, len(runtimeCounters))
	for _, counter := range runtimeCounters {
		snapshot[counter.name] = counter.value(r).Load()
	}
	return snapshot
}

// runtimeCounters names every counter in the Snapshot view.
var runtimeCounters = []struct {
	name  string
	value func(*Runtime) *atomic.Uint64
}{
	{"agentic_stream_events_ingested_total", func(r *Runtime) *atomic.Uint64 { return &r.eventsIngested }},
	{"agentic_stream_events_processed_total", func(r *Runtime) *atomic.Uint64 { return &r.eventsProcessed }},
	{"agentic_stream_episodes_admitted_total", func(r *Runtime) *atomic.Uint64 { return &r.episodesAdmitted }},
	{"agentic_stream_episodes_executed_total", func(r *Runtime) *atomic.Uint64 { return &r.episodesExecuted }},
	{"agentic_stream_intents_evaluated_total", func(r *Runtime) *atomic.Uint64 { return &r.intentsEvaluated }},
	{"agentic_stream_commands_dispatched_total", func(r *Runtime) *atomic.Uint64 { return &r.commandsDispatched }},
	{"agentic_stream_pipeline_failures_total", func(r *Runtime) *atomic.Uint64 { return &r.streamFailures }},
	{"agentic_stream_stale_rejections_total", func(r *Runtime) *atomic.Uint64 { return &r.staleRejections }},
	{"agentic_stream_stale_rebinds_total", func(r *Runtime) *atomic.Uint64 { return &r.staleRebinds }},
	{"agentic_stream_rebind_failures_total", func(r *Runtime) *atomic.Uint64 { return &r.rebindFailures }},
	{"agentic_stream_device_frame_errors_total", func(r *Runtime) *atomic.Uint64 { return &r.deviceFrameErrors }},
	{"agentic_stream_device_reconnects_total", func(r *Runtime) *atomic.Uint64 { return &r.deviceReconnects }},
	{"agentic_stream_action_unknown_outcomes_total", func(r *Runtime) *atomic.Uint64 { return &r.actionUnknownOutcomes }},
	{"agentic_stream_verification_pending_total", func(r *Runtime) *atomic.Uint64 { return &r.verificationPending }},
	{"agentic_stream_verification_failures_total", func(r *Runtime) *atomic.Uint64 { return &r.verificationFailures }},
	{"agentic_stream_lease_expiries_total", func(r *Runtime) *atomic.Uint64 { return &r.leaseExpiries }},
	{"agentic_stream_safe_state_entries_total", func(r *Runtime) *atomic.Uint64 { return &r.safeStateEntries }},
	{"agentic_stream_reconciliation_barriers_total", func(r *Runtime) *atomic.Uint64 { return &r.reconciliationBarriers }},
	{"agentic_stream_target_claim_rejections_total", func(r *Runtime) *atomic.Uint64 { return &r.targetClaimRejections }},
	{"agentic_stream_safe_stop_requests_total", func(r *Runtime) *atomic.Uint64 { return &r.safeStopRequests }},
	{"agentic_stream_safe_stop_failures_total", func(r *Runtime) *atomic.Uint64 { return &r.safeStopFailures }},
	{"agentic_stream_safe_stop_completions_total", func(r *Runtime) *atomic.Uint64 { return &r.safeStopCompletions }},
	{"agentic_stream_live_lines_ingested_total", func(r *Runtime) *atomic.Uint64 { return &r.liveLinesIngested }},
	{"agentic_stream_live_lines_rejected_total", func(r *Runtime) *atomic.Uint64 { return &r.liveLinesRejected }},
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
