package transport

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Sources owns source adapter creation, not pipeline sequencing.
type Sources struct {
	DB        *storage.DB
	Log       *eventlog.EventLog
	TenantID  string
	Telemetry *telemetry.Runtime
}

// RunJSONL ingests normalized trace records through the existing durable adapter.
func (s *Sources) RunJSONL(ctx context.Context, path string) (int, error) {
	return ingress.NewJSONLReplay(s.DB, s.Log, s.TenantID, path, "live-jsonl:"+path).Run(ctx) //nolint:wrapcheck // App supplies operation context; preserve original ingress errors.
}

// RunSimulatorJSONL ingests simulator records through its strict adapter.
func (s *Sources) RunSimulatorJSONL(ctx context.Context, path string) (int, error) {
	return ingress.NewSimulatorJSONLReplay(s.DB, s.Log, ingress.SimulatorOptions{TenantID: s.TenantID}, path, "live-simulator:"+path).Run(ctx) //nolint:wrapcheck // App supplies operation context; preserve original ingress errors.
}

// RunLiveSocket owns the live UDS source and forwards accepted envelopes to app.
func (s *Sources) RunLiveSocket(ctx context.Context, path string, sink func(context.Context, contractsv1.Envelope) error) error {
	source := ingress.NewLiveUDSSource(s.Log, s.TenantID, path).WithTelemetry(s.Telemetry)
	return source.Run(ctx, sink) //nolint:wrapcheck // App classifies cancellation and supplies source operation context.
}
