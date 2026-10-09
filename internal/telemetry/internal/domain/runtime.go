package domain

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"
)

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

	staleRejections atomic.Uint64

	staleRebinds atomic.Uint64

	rebindFailures         atomic.Uint64
	deviceFrameErrors      atomic.Uint64
	actionUnknownOutcomes  atomic.Uint64
	verificationPending    atomic.Uint64
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

func NewRuntime(now time.Time, tracer trace.Tracer) *Runtime {
	return &Runtime{started: now.UTC(), tracer: tracer}
}

func (r *Runtime) StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	if r == nil || r.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return r.tracer.Start(ctx, name, options...)
}

type PipelineReport struct {
	EventsIngested     int
	EventsProcessed    int
	EpisodesAdmitted   int
	EpisodesExecuted   int
	IntentsEvaluated   int
	CommandsDispatched int
}

func (r *Runtime) ObserveDuration(duration time.Duration) {
	if r == nil {
		return
	}
	r.durationsMu.Lock()
	defer r.durationsMu.Unlock()
	r.durations = append(r.durations, duration)
	if excess := len(r.durations) - MaxRecentDurations; excess > 0 {
		r.durations = r.durations[excess:]
	}
}

const MaxRecentDurations = 1024

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
	{"agentic_stream_action_unknown_outcomes_total", func(r *Runtime) *atomic.Uint64 { return &r.actionUnknownOutcomes }},
	{"agentic_stream_verification_pending_total", func(r *Runtime) *atomic.Uint64 { return &r.verificationPending }},
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

func latencyNanos(d time.Duration) uint64 {
	if d < 0 {
		return 0
	}
	return uint64(d)
}

func (r *Runtime) LatencySnapshot() map[string]uint64 {
	return map[string]uint64{
		"agentic_stream_dispatch_decision_p50_ns": latencyNanos(r.Percentile(50)),
		"agentic_stream_dispatch_decision_p95_ns": latencyNanos(r.Percentile(95)),
		"agentic_stream_dispatch_decision_p99_ns": latencyNanos(r.Percentile(99)),
	}
}
