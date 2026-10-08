package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// EnvelopeSink receives one validated normalized envelope from a live source.
// The sink owns appending the envelope and advancing the runtime pipeline.
type EnvelopeSink func(context.Context, contractsv1.Envelope) error

// Telemetry counts accepted and rejected live lines.
type Telemetry interface {
	ObserveLiveLineIngested()
	ObserveLiveLineRejected()
}

// Config supplies the checkpoint store and event log, which are required, and
// the optional clock, tenant, logger, telemetry and live queue size.
type Config struct {
	Store         store.Store
	Log           *eventlog.EventLog
	Clock         sources.Clock
	TenantID      string
	Logger        *slog.Logger
	Telemetry     Telemetry
	LiveQueueSize int
}

// Service reads external sources into the event log.
type Service struct {
	store     store.Store
	log       *eventlog.EventLog
	clk       sources.Clock
	tenantID  string
	logger    *slog.Logger
	telemetry Telemetry
	queueSize int
}

// New validates the configuration and applies the clock, tenant, logger and
// queue defaults.
func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() || cfg.Log == nil {
		return nil, errors.New("ingress requires a database and an event log")
	}
	return &Service{store: cfg.Store, log: cfg.Log, clk: sources.OrPhysical(cfg.Clock), tenantID: domain.TenantOrDefault(cfg.TenantID),
		logger: orDefaultLogger(cfg.Logger), telemetry: cfg.Telemetry, queueSize: orDefaultQueue(cfg.LiveQueueSize)}, nil
}

func orDefaultLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func orDefaultQueue(size int) int {
	if size <= 0 {
		return domain.DefaultLiveQueueSize
	}
	return size
}
