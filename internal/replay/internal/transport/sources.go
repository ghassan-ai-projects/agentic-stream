package transport

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
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
func (s Sources) RunJSONLTrace(ctx context.Context, path string, clk *clock.Virtual) (int, error) {
	conn := ingress.NewJSONLReplayWithClock(s.DB, s.Log, s.TenantID, path, "replay:"+path, clk)
	return conn.Run(ctx) //nolint:wrapcheck // App wraps with the session operation name; preserve original ingress errors.
}
