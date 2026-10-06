// Package evidence exposes configured, bounded read-tool and recovery use cases.
package evidence

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// CapabilityConfig defines the immutable issuing/verifying authority.
type CapabilityConfig = app.CapabilityConfig

// LedgerConfig binds durable calls to one runtime owner.
type LedgerConfig struct {
	DB                       *storage.DB
	OwnerCheck               func(context.Context, *sql.Tx, string) error
	LeaseOwner, RuntimeEpoch string
	Lease                    time.Duration
	Now                      func() time.Time
}

// CallConfig binds the existing capability and ledger services to a read-only query.
type CallConfig struct {
	Capabilities, Ledger *Service
	Query                Query
	RuntimeEpoch         string
	Now                  func() time.Time
}

// Config explicitly selects capability, ledger or worker-call use cases.
type Config struct {
	Capabilities *CapabilityConfig
	Ledger       *LedgerConfig
	Calls        *CallConfig
}

// New validates configured dependencies before any evidence operation runs.
func New(cfg Config) (*Service, error) {
	service, err := app.New(applicationConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &Service{app: service, transport: transport.New(service)}, nil
}
func applicationConfig(cfg Config) app.Config {
	return app.Config{Capabilities: cfg.Capabilities, Ledger: ledgerConfig(cfg.Ledger), Calls: callConfig(cfg.Calls)}
}
func ledgerConfig(cfg *LedgerConfig) *app.Ledger {
	if cfg == nil {
		return nil
	}
	return &app.Ledger{Store: store.New(cfg.DB, cfg.OwnerCheck, cfg.RuntimeEpoch), LeaseOwner: cfg.LeaseOwner, RuntimeEpoch: cfg.RuntimeEpoch, Lease: cfg.Lease, Now: cfg.Now}
}
func callConfig(cfg *CallConfig) *app.CallConfig {
	if cfg == nil {
		return nil
	}
	return &app.CallConfig{Capabilities: privateService(cfg.Capabilities), Ledger: privateService(cfg.Ledger), Query: cfg.Query, RuntimeEpoch: cfg.RuntimeEpoch, Now: cfg.Now}
}
func privateService(service *Service) *app.Service {
	if service == nil {
		return nil
	}
	return service.app
}
