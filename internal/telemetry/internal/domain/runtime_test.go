package domain

import (
	"context"
	"maps"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var observations = []struct {
	counter string
	observe func(*Runtime)
}{
	{"agentic_stream_pipeline_failures_total", (*Runtime).ObserveFailure},
	{"agentic_stream_stale_rejections_total", (*Runtime).ObserveStaleRejection},
	{"agentic_stream_stale_rebinds_total", (*Runtime).ObserveStaleRebind},
	{"agentic_stream_rebind_failures_total", (*Runtime).ObserveRebindFailure},
	{"agentic_stream_device_frame_errors_total", (*Runtime).ObserveDeviceFrameError},
	{"agentic_stream_action_unknown_outcomes_total", (*Runtime).ObserveActionUnknownOutcome},
	{"agentic_stream_verification_pending_total", (*Runtime).ObserveVerificationPending},
	{"agentic_stream_lease_expiries_total", (*Runtime).ObserveLeaseExpiry},
	{"agentic_stream_safe_state_entries_total", (*Runtime).ObserveSafeStateEntry},
	{"agentic_stream_reconciliation_barriers_total", (*Runtime).ObserveReconciliationBarrier},
	{"agentic_stream_target_claim_rejections_total", (*Runtime).ObserveTargetClaimRejection},
	{"agentic_stream_safe_stop_requests_total", (*Runtime).ObserveSafeStopRequested},
	{"agentic_stream_safe_stop_failures_total", (*Runtime).ObserveSafeStopFailure},
	{"agentic_stream_safe_stop_completions_total", (*Runtime).ObserveSafeStopCompleted},
	{"agentic_stream_live_lines_ingested_total", (*Runtime).ObserveLiveLineIngested},
	{"agentic_stream_live_lines_rejected_total", (*Runtime).ObserveLiveLineRejected},
}

func TestEachObservationIncrementsOnlyItsOwnCounter(t *testing.T) {
	t.Parallel()
	for _, tc := range observations {
		t.Run(tc.counter, func(t *testing.T) {
			t.Parallel()
			runtime := NewRuntime(time.Unix(1, 0), nil)

			tc.observe(runtime)
			tc.observe(runtime)

			for name, value := range runtime.Snapshot() {
				want := uint64(0)
				if name == tc.counter {
					want = 2
				}
				if value != want {
					t.Errorf("%s = %d after observing %s twice, want %d", name, value, tc.counter, want)
				}
			}
		})
	}
}

func TestPipelineReportAddsEachFieldToItsOwnCounter(t *testing.T) {
	t.Parallel()
	runtime := NewRuntime(time.Unix(1, 0), nil)

	runtime.ObservePipeline(PipelineReport{EventsIngested: 1, EventsProcessed: 2, EpisodesAdmitted: 3, EpisodesExecuted: 4, IntentsEvaluated: 5, CommandsDispatched: 6})
	runtime.ObservePipeline(PipelineReport{EventsIngested: 10, EventsProcessed: -7, EpisodesAdmitted: 0})

	want := map[string]uint64{
		"agentic_stream_events_ingested_total":     11,
		"agentic_stream_events_processed_total":    2,
		"agentic_stream_episodes_admitted_total":   3,
		"agentic_stream_episodes_executed_total":   4,
		"agentic_stream_intents_evaluated_total":   5,
		"agentic_stream_commands_dispatched_total": 6,
	}
	snapshot := runtime.Snapshot()
	for name, value := range want {
		if snapshot[name] != value {
			t.Errorf("%s = %d, want %d (negative and zero counts are ignored)", name, snapshot[name], value)
		}
	}
}

func TestSnapshotNamesEveryCounterExactlyOnce(t *testing.T) {
	t.Parallel()
	names := map[string]struct{}{
		"agentic_stream_events_ingested_total":     {},
		"agentic_stream_events_processed_total":    {},
		"agentic_stream_episodes_admitted_total":   {},
		"agentic_stream_episodes_executed_total":   {},
		"agentic_stream_intents_evaluated_total":   {},
		"agentic_stream_commands_dispatched_total": {},
	}
	for _, tc := range observations {
		names[tc.counter] = struct{}{}
	}

	snapshot := NewRuntime(time.Unix(1, 0), nil).Snapshot()

	if len(snapshot) != len(names) || len(runtimeCounters) != len(names) {
		t.Fatalf("snapshot has %d counters and %d definitions, want %d", len(snapshot), len(runtimeCounters), len(names))
	}
	for name, value := range snapshot {
		if _, known := names[name]; !known || value != 0 {
			t.Errorf("snapshot counter %s = %d, known=%t; want a known counter at 0", name, value, known)
		}
	}
}

func TestNilRuntimeIsInert(t *testing.T) {
	t.Parallel()
	var runtime *Runtime

	for _, tc := range observations {
		tc.observe(runtime)
	}
	runtime.ObservePipeline(PipelineReport{EventsIngested: 1})
	runtime.ObserveDuration(time.Second)

	if runtime.Snapshot() != nil {
		t.Fatal("nil runtime reported counters")
	}
	if got := runtime.Percentile(50); got != 0 {
		t.Fatalf("nil runtime reported a latency of %v", got)
	}
	if got := runtime.LatencySnapshot()["agentic_stream_dispatch_decision_p99_ns"]; got != 0 {
		t.Fatalf("nil runtime reported a p99 of %d", got)
	}
}

func TestRuntimeSpansAreRecordedOnlyWhenATracerIsConfigured(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.WithoutCancel(t.Context())) })

	configured := NewRuntime(time.Unix(1, 0), provider.Tracer("test"))
	_, span := configured.StartSpan(t.Context(), "pipeline")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "pipeline" {
		t.Fatalf("exported spans = %v, want one named pipeline", spans)
	}
	for name, runtime := range map[string]*Runtime{"nil runtime": nil, "nil tracer": NewRuntime(time.Unix(1, 0), nil)} {
		ctx, span := runtime.StartSpan(t.Context(), "ignored")
		if span.SpanContext().IsValid() || ctx != t.Context() {
			t.Errorf("%s started a span (valid=%t) or replaced the context", name, span.SpanContext().IsValid())
		}
	}
	if got := len(exporter.GetSpans()); got != 1 {
		t.Fatalf("exported spans = %d, want only the configured runtime's span", got)
	}
}

func TestSnapshotsAreIndependentCopies(t *testing.T) {
	t.Parallel()
	runtime := NewRuntime(time.Unix(1, 0), nil)
	first := runtime.Snapshot()

	runtime.ObserveFailure()

	if first["agentic_stream_pipeline_failures_total"] != 0 || runtime.Snapshot()["agentic_stream_pipeline_failures_total"] != 1 {
		t.Fatalf("a snapshot changed after it was taken: %v", maps.Clone(first))
	}
}
