package engine

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Config supplies the safety dependencies an engine requires. DB, Log, Spec and
// RuntimeOwner are required: a missing one is a constructor error, never a
// silently skipped check. RuntimeOwner is the ownership assertion for Epoch, run
// on every stream transaction; control.RuntimeOwner.Assert is the production
// check and ReplayOwnership is the explicit choice for deterministic replay and
// tests. Clock defaults to the physical clock and TenantID to the default
// tenant. Cognition selects whether Situation versions reach cognition.
type Config struct {
	DB           *storage.DB
	Log          *eventlog.EventLog
	Clock        clock.Clock
	Spec         *spec.CompiledSpec
	TenantID     string
	RuntimeOwner func(context.Context, *sql.Tx, string) error
	Epoch        string
	Cognition    bool
}

// ReplayOwnership is the ownership check for runs that have no runtime owner:
// deterministic replay and tests. It asserts nothing.
func ReplayOwnership(context.Context, *sql.Tx, string) error { return nil }

// New validates the configuration, records the deployment, restores durable
// Situations and composes the engine use cases.
func New(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Spec == nil {
		return nil, errors.New("engine requires a compiled spec")
	}
	service, err := app.New(ctx, applicationConfig(cfg))
	if err != nil {
		return nil, err //nolint:wrapcheck // The app layer's constructor errors are the facade's contract.
	}
	return &Service{app: service}, nil
}

func applicationConfig(cfg Config) app.Config {
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = contractsv1.TenantID
	}
	return app.Config{Store: store.New(cfg.DB, cfg.RuntimeOwner, cfg.Epoch, tenantID, cfg.Spec.Digest),
		Log: cfg.Log, Clock: cfg.Clock, Spec: cfg.Spec, TenantID: tenantID, Cognition: cfg.Cognition}
}
