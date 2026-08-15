package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md, exit gate 6): latency/freshness
// SLOs are measured and exported — the dispatch→decision percentiles and the
// stale-decision rejection rate appear in the /metrics payload.

func TestP8LatencyPercentilesAndStaleRejectionsAreMeasured(t *testing.T) {
	r := NewRuntime(time.Now().UTC())
	for i := 1; i <= 100; i++ {
		r.ObserveDuration(time.Duration(i) * time.Millisecond)
	}
	if got := r.Percentile(50); got != 50*time.Millisecond {
		t.Fatalf("p50 = %v, want 50ms", got)
	}
	if got := r.Percentile(95); got != 95*time.Millisecond {
		t.Fatalf("p95 = %v, want 95ms", got)
	}
	if got := r.Percentile(99); got != 99*time.Millisecond {
		t.Fatalf("p99 = %v, want 99ms", got)
	}
	r.ObserveStaleRejection()
	r.ObserveStaleRejection()
	if r.Snapshot()["agentic_stream_stale_rejections_total"] != 2 {
		t.Fatalf("stale rejection counter = %d, want 2",
			r.Snapshot()["agentic_stream_stale_rejections_total"])
	}
}

func TestP8LatencySnapshotAppearsInHandlerPayload(t *testing.T) {
	r := NewRuntime(time.Now().UTC())
	r.ObserveDuration(10 * time.Millisecond)
	response := httptest.NewRecorder()
	r.Handler().ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if !strings.Contains(body, "agentic_stream_dispatch_decision_p95_ns") {
		t.Fatalf("p95 must appear in the metrics payload:\n%s", body)
	}
	if !strings.Contains(body, "agentic_stream_stale_rejections_total") {
		t.Fatalf("stale-rejection counter must appear in the metrics payload:\n%s", body)
	}
}
