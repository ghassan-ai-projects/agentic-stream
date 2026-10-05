package transport

import (
	"context"
	"fmt"
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
	count, err := ingress.NewJSONLReplay(s.DB, s.Log, s.TenantID, path, "live-jsonl:"+path).Run(ctx)
	if err != nil {
		return count, fmt.Errorf("%w", err)
	}
	return count, nil
}

// RunSimulatorJSONL ingests simulator records through its strict adapter.
func (s *Sources) RunSimulatorJSONL(ctx context.Context, path string) (int, error) {
	count, err := ingress.NewSimulatorJSONLReplay(s.DB, s.Log, ingress.SimulatorOptions{TenantID: s.TenantID}, path, "live-simulator:"+path).Run(ctx)
	if err != nil {
		return count, fmt.Errorf("%w", err)
	}
	return count, nil
}

// RunLiveSocket owns the live UDS source and forwards accepted envelopes to app.
func (s *Sources) RunLiveSocket(ctx context.Context, path string, sink func(context.Context, contractsv1.Envelope) error) error {
	source := ingress.NewLiveUDSSource(s.Log, s.TenantID, path).WithTelemetry(s.Telemetry)
	if err := source.Run(ctx, sink); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
