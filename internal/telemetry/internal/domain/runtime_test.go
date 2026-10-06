package domain

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
)

func TestEveryObservationIncrementsItsCounter(t *testing.T) {
	t.Parallel()
	r := NewRuntime(time.Unix(1, 0), nil)
	observations := []func(){
		r.ObserveFailure, r.ObserveStaleRejection, r.ObserveStaleRebind, r.ObserveRebindFailure,
		r.ObserveDeviceFrameError, r.ObserveDeviceReconnect, r.ObserveActionUnknownOutcome,
		r.ObserveVerificationPending, r.ObserveVerificationFailure, r.ObserveLeaseExpiry,
		r.ObserveSafeStateEntry, r.ObserveReconciliationBarrier, r.ObserveTargetClaimRejection,
		r.ObserveSafeStopRequested, r.ObserveSafeStopFailure, r.ObserveSafeStopCompleted,
		r.ObserveLiveLineIngested, r.ObserveLiveLineRejected,
	}
	for _, observe := range observations {
		observe()
	}
	r.ObservePipeline(PipelineReport{EventsIngested: 2, CommandsDispatched: 1})
	var total uint64
	for _, value := range r.Snapshot() {
		total += value
	}
	if want := uint64(len(observations)) + 3; total != want {
		t.Fatalf("counter total = %d, want %d", total, want)
	}
}

func TestNilRuntimeIsInertAndSpansFallBackToTheContext(t *testing.T) {
	t.Parallel()
	var r *Runtime
	r.ObserveFailure()
	r.ObserveDuration(time.Second)
	if r.Percentile(50) != 0 {
		t.Fatal("nil runtime reported a latency")
	}
	ctx, span := r.StartSpan(context.Background(), "x")
	if ctx == nil || span == nil {
		t.Fatal("nil runtime returned no span")
	}
	traced := NewRuntime(time.Unix(1, 0), noop.NewTracerProvider().Tracer("t"))
	if _, span := traced.StartSpan(context.Background(), "y"); span == nil {
		t.Fatal("configured runtime returned no span")
	}
}

func TestPercentileOrdersRecordedDurations(t *testing.T) {
	t.Parallel()
	r := NewRuntime(time.Unix(1, 0), nil)
	for _, d := range []time.Duration{30, 10, 20} {
		r.ObserveDuration(d)
	}
	if got := r.Percentile(100); got != 30 {
		t.Fatalf("p100 = %v", got)
	}
	if got := r.Percentile(0); got != 10 {
		t.Fatalf("p0 = %v", got)
	}
}
