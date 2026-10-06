package transport

import (
	"fmt"
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/domain"
)

// MetricsHandler exposes low-cardinality Prometheus text without leaking tenant
// or event data. It is safe to mount behind the same local HTTP listener.
func MetricsHandler(r *domain.Runtime) http.Handler {
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
