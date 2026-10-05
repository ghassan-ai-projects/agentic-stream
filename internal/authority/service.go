package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// defaultClaimLease is how long a target claim stays live without renewal
// when Config.ClaimLease is zero.
const defaultClaimLease = time.Minute

// OutcomeLedger counts, inside tx, the commands among commandIDs whose
// outcome is still unresolved. actions.CountUnresolvedOutcomes implements it;
// the action ledger owns what "unresolved" means.
type OutcomeLedger func(ctx context.Context, tx *sql.Tx, commandIDs []string) (int64, error)

// Config wires a Service. DB, Owner, Epochs and Outcomes are safety
// dependencies and are required; they must all use the same database.
type Config struct {
	DB         *storage.DB
	Owner      *control.RuntimeOwner
	Epochs     *control.EpochControl
	Outcomes   OutcomeLedger
	ClaimLease time.Duration
	Clock      clock.Clock
}

// Service is the device-authority module's only entry point. It delegates
// every operation to the module's use cases.
type Service struct {
	app *app.Service
}

// New returns a Service, refusing a configuration that would silently skip
// a safety check.
func New(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Service{app: app.New(app.Config{
		Store:         store.New(cfg.DB),
		Fences:        app.Fences{RuntimeOwner: cfg.Owner.Assert, EpochControl: cfg.Epochs.AssertOrdinaryTx},
		Outcomes:      store.OutcomeLedger(cfg.Outcomes),
		OwnerInstance: cfg.Owner.InstanceID,
		ClaimLease:    cfg.claimLease(),
		Clock:         cfg.clock(),
	})}, nil
}

func (cfg Config) validate() error {
	if cfg.DB == nil || cfg.Owner == nil || cfg.Epochs == nil || cfg.Outcomes == nil {
		return errors.New("device authority requires a database, runtime owner, epoch control and outcome ledger")
	}
	if cfg.Owner.DB != cfg.DB || cfg.Epochs.DB != cfg.DB {
		return errors.New("device authority, runtime owner and epoch control must share one database")
	}
	if cfg.Owner.InstanceID == "" {
		return errors.New("device authority requires a runtime owner instance")
	}
	if cfg.ClaimLease < 0 {
		return fmt.Errorf("device authority claim lease %s is negative", cfg.ClaimLease)
	}
	return nil
}

func (cfg Config) claimLease() time.Duration {
	if cfg.ClaimLease == 0 {
		return defaultClaimLease
	}
	return cfg.ClaimLease
}

func (cfg Config) clock() clock.Clock {
	if cfg.Clock == nil {
		return clock.Physical()
	}
	return cfg.Clock
}
