// Package actions is the governed dispatcher after policy approval: it leases
// approved commands, dispatches them through effect ports, and verifies and
// reconciles their outcomes. Concrete effect adapters live elsewhere. See the
// [module guide](README.md).
package actions

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Config supplies the safety dependencies a dispatcher requires and the
// replaceable clock, identity, lease and telemetry settings.
type Config struct {
	// DB, Effector and RuntimeOwner are required: a missing one is a
	// constructor error, never a silently skipped check. RuntimeOwner is the
	// ownership assertion for Epoch, run on the dispatch transaction;
	// control.RuntimeOwner.Assert is the production check.
	DB           *storage.DB
	Effector     actionport.AuthorizedEffector
	RuntimeOwner func(context.Context, *sql.Tx, string) error
	Epoch        string
	// Clock, IDs, LeaseOwner and LeaseFor default to the physical clock, random
	// identities, "actions" and one minute.
	Clock      sources.Clock
	IDs        sources.Generator
	LeaseOwner string
	LeaseFor   time.Duration
	// Telemetry is optional.
	Telemetry *telemetry.Runtime
}

// Service is the public facade over the action use cases.
type Service struct{ app *app.Service }

// New validates the configuration and composes the action use cases.
func New(cfg Config) (*Service, error) {
	service, err := app.New(app.Config{
		Store:    store.New(cfg.DB, cfg.RuntimeOwner, cfg.Epoch),
		Effector: cfg.Effector, Clock: cfg.Clock, IDs: cfg.IDs, LeaseOwner: cfg.LeaseOwner, LeaseFor: cfg.LeaseFor,
		Observer: leaseObserver(cfg.Telemetry),
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // The app layer's constructor errors are the facade's contract.
	}
	return &Service{app: service}, nil
}

func leaseObserver(runtimeTelemetry *telemetry.Runtime) app.LeaseObserver {
	if runtimeTelemetry == nil {
		return nil
	}
	return runtimeTelemetry
}

// DispatchOnce processes at most one command: lease, revalidate, dispatch,
// verify independently when supported, and record the outcome atomically.
func (s *Service) DispatchOnce(ctx context.Context) (bool, error) { return s.app.DispatchOnce(ctx) }

// CountUnresolvedOutcomes counts, inside tx, the commands among commandIDs
// whose outcome still awaits reconciliation. The device authority uses it to
// refuse clearing a device while one of its commands is unresolved.
func CountUnresolvedOutcomes(ctx context.Context, tx *sql.Tx, commandIDs []string) (int64, error) {
	return store.JoinCaller(tx).CountUnresolvedOutcomes(ctx, commandIDs)
}
