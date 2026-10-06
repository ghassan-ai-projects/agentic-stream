package ingress

import (
	"context"
	"log/slog"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// EnvelopeSink receives one validated normalized envelope from a live source.
// The sink owns appending the envelope and advancing the runtime pipeline.
type EnvelopeSink = app.EnvelopeSink

// SimulatorOptions describes the consumer-specific projection from the
// streams-simulator trace-record-v0.1 format into current normalized events.
type SimulatorOptions = domain.SimulatorOptions

// Config supplies the required database and event log and the optional clock,
// tenant, logger, telemetry and live queue size. Clock defaults to the physical
// clock and TenantID to the default tenant.
type Config struct {
	DB            *storage.DB
	Log           *eventlog.EventLog
	Clock         sources.Clock
	TenantID      string
	Logger        *slog.Logger
	Telemetry     *telemetry.Runtime
	LiveQueueSize int
}

// Service is the public facade over the ingress use cases.
type Service struct{ app *app.Service }

// New validates the configuration and composes the ingress use cases.
func New(cfg Config) (*Service, error) {
	service, err := app.New(applicationConfig(cfg))
	if err != nil {
		return nil, err //nolint:wrapcheck // The app layer's constructor errors are the facade's contract.
	}
	return &Service{app: service}, nil
}

func applicationConfig(cfg Config) app.Config {
	acfg := app.Config{Store: store.New(cfg.DB), Log: cfg.Log, Clock: cfg.Clock, TenantID: cfg.TenantID, Logger: cfg.Logger, LiveQueueSize: cfg.LiveQueueSize}
	if cfg.Telemetry != nil {
		acfg.Telemetry = cfg.Telemetry
	}
	return acfg
}

// ReplayJSONL appends the remaining envelopes of a JSON Lines trace from the
// connector's checkpoint and reports how many were new. An empty connector ID
// names the connector after the path.
func (s *Service) ReplayJSONL(ctx context.Context, path, connectorID string) (int, error) {
	return s.app.ReplayJSONL(ctx, path, connectorID)
}

// ReplaySimulator appends the new event records of a streams-simulator trace
// from the connector's checkpoint and reports how many were new.
func (s *Service) ReplaySimulator(ctx context.Context, options SimulatorOptions, path, connectorID string) (int, error) {
	return s.app.ReplaySimulator(ctx, options, path, connectorID)
}

// ServeLive listens on the Unix socket at path and hands each admitted envelope
// to sink until ctx is canceled or the sink fails.
func (s *Service) ServeLive(ctx context.Context, path string, sink EnvelopeSink) error {
	return s.app.ServeLive(ctx, path, sink)
}
