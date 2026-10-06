package transport

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Sources owns the trace ingestion adapter, not session sequencing.
type Sources struct {
	DB       *storage.DB
	Log      *eventlog.EventLog
	TenantID string
}

// RunJSONLTrace ingests the normalized trace through the ingress replay
// adapter with the session's virtual clock.
func (s Sources) RunJSONLTrace(ctx context.Context, path string, clk *sources.Virtual) (int, error) {
	service, err := ingress.New(ingress.Config{DB: s.DB, Log: s.Log, Clock: clk, TenantID: s.TenantID})
	if err != nil {
		return 0, err //nolint:wrapcheck // Ingress names the missing dependency.
	}
	return service.ReplayJSONL(ctx, path, "replay:"+path) //nolint:wrapcheck // App wraps with the session operation name; preserve original ingress errors.
}
