package runtime

import (
	"context"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
)

// Service is the public readiness and runtime-lease facade.
type Service struct{ application *app.Service }

// NewService requires an owner, evidence ledger and bound epoch.
func NewService(owner *runtimecontrol.RuntimeOwner, ledger *evidence.Ledger, epoch string) (*Service, error) {
	application, err := app.NewService(owner, ledger, epoch)
	if err != nil {
		return nil, err
	}
	return &Service{application: application}, nil
}
func (s *Service) useCases() *app.Service {
	if s == nil {
		return nil
	}
	return s.application
}

// Start recovers work and starts the heartbeat before readiness.
func (s *Service) Start(ctx context.Context) (RecoveryReport, error) { return s.useCases().Start(ctx) }

// Ready reports readiness maintained by the app lifecycle.
func (s *Service) Ready() error { return s.useCases().Ready() }

// Close stops heartbeat and releases the runtime lease.
func (s *Service) Close(ctx context.Context) error { return s.useCases().Close(ctx) }
