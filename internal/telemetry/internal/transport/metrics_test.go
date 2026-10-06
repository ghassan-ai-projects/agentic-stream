package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/domain"
)

func TestP8LatencySnapshotAppearsInHandlerPayload(t *testing.T) {
	r := domain.NewRuntime(time.Now().UTC(), nil)
	r.ObserveDuration(10 * time.Millisecond)
	response := httptest.NewRecorder()
	MetricsHandler(r).ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	if !strings.Contains(body, "agentic_stream_dispatch_decision_p95_ns") {
		t.Fatalf("p95 must appear in the metrics payload:\n%s", body)
	}
	if !strings.Contains(body, "agentic_stream_stale_rejections_total") {
		t.Fatalf("stale-rejection counter must appear in the metrics payload:\n%s", body)
	}
	if !strings.Contains(body, "agentic_stream_stale_rebinds_total") {
		t.Fatalf("stale re-bind counter must appear in the metrics payload:\n%s", body)
	}
	if !strings.Contains(body, "agentic_stream_rebind_failures_total") {
		t.Fatalf("re-bind failure counter must appear in the metrics payload:\n%s", body)
	}
}
