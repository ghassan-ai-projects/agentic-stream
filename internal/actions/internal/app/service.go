package app

import (
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// LeaseObserver counts dispatch leases that expired before a result was recorded.
type LeaseObserver interface{ ObserveLeaseExpiry() }

// Config supplies the safety dependencies every dispatch needs and the
// replaceable clock, identity, lease and telemetry settings.
type Config struct {
	// Store carries the database and runtime ownership check. It must
	// be fully configured.
	Store store.Store
	// Effector must enforce the final dispatch authorization check at the
	// effect boundary; an effector that cannot is refused at construction.
	Effector actionport.AuthorizedEffector
	Clock    sources.Clock
	IDs      sources.Generator
	// LeaseOwner names this dispatcher in lease rows.
	LeaseOwner string
	LeaseFor   time.Duration
	// Observer is optional telemetry.
	Observer LeaseObserver
}

// Service owns the configured action use cases.
type Service struct {
	store    store.Store
	effector actionport.AuthorizedEffector
	clk      sources.Clock
	ids      sources.Generator
	owner    string
	leaseFor time.Duration
	observer LeaseObserver
}

// New validates the safety dependencies and applies clock, identity and lease
// defaults, none of which bypass authorization.
func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() {
		return nil, errors.New("actions require a database and runtime owner check")
	}
	if cfg.Effector == nil {
		return nil, errors.New("actions require an effector")
	}
	return &Service{store: cfg.Store, effector: cfg.Effector, clk: sources.OrPhysical(cfg.Clock), ids: sources.OrRandom(cfg.IDs),
		owner: orDefault(cfg.LeaseOwner, "actions"), leaseFor: orMinute(cfg.LeaseFor), observer: cfg.Observer}, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func orMinute(lease time.Duration) time.Duration {
	if lease <= 0 {
		return time.Minute
	}
	return lease
}
