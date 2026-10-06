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

func (s *Sources) service() (*ingress.Service, error) {
	return ingress.New(ingress.Config{DB: s.DB, Log: s.Log, TenantID: s.TenantID, Telemetry: s.Telemetry}) //nolint:wrapcheck // Ingress names the missing dependency.
}

// RunJSONL ingests normalized trace records through the durable adapter.
func (s *Sources) RunJSONL(ctx context.Context, path string) (int, error) {
	service, err := s.service()
	if err != nil {
		return 0, err
	}
	return service.ReplayJSONL(ctx, path, "live-jsonl:"+path) //nolint:wrapcheck // App supplies operation context; preserve original ingress errors.
}

// RunSimulatorJSONL ingests simulator records through its strict adapter.
func (s *Sources) RunSimulatorJSONL(ctx context.Context, path string) (int, error) {
	service, err := s.service()
	if err != nil {
		return 0, err
	}
	return service.ReplaySimulator(ctx, ingress.SimulatorOptions{TenantID: s.TenantID}, path, "live-simulator:"+path) //nolint:wrapcheck // App supplies operation context; preserve original ingress errors.
}

// RunLiveSocket serves the live socket source and forwards accepted envelopes to app.
func (s *Sources) RunLiveSocket(ctx context.Context, path string, sink func(context.Context, contractsv1.Envelope) error) error {
	service, err := s.service()
	if err != nil {
		return err
	}
	return service.ServeLive(ctx, path, sink) //nolint:wrapcheck // App classifies cancellation and supplies source operation context.
}
