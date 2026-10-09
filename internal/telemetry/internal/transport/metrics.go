package transport

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry/internal/domain"
)

type GaugeSource func(context.Context) (map[string]float64, error)

func MetricsHandler(r *domain.Runtime, gauges GaugeSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		writeFamily(w, "counter", r.Snapshot())
		writeFamily(w, "gauge", r.LatencySnapshot())
		writeGauges(req.Context(), w, gauges)
	})
}

func writeFamily(w io.Writer, kind string, values map[string]uint64) {
	for _, name := range slices.Sorted(maps.Keys(values)) {
		_, _ = fmt.Fprintf(w, "# TYPE %s %s\n%s %d\n", name, kind, name, values[name])
	}
}

func writeGauges(ctx context.Context, w io.Writer, gauges GaugeSource) {
	if gauges == nil {
		return
	}
	values, err := gauges(ctx)
	if err != nil {
		slog.Warn("metrics gauges unavailable", "error", err)
		return
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		_, _ = fmt.Fprintf(w, "# TYPE %s gauge\n%s %g\n", name, name, values[name])
	}
}
