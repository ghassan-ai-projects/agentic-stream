package app

import (
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// LeaseObserver counts dispatch leases that expired before a result was recorded.
type LeaseObserver interface{ ObserveLeaseExpiry() }

// Config supplies the safety dependencies every dispatch needs and the
// replaceable clock, identity, lease and telemetry settings.
type Config struct {
	// Store carries the database, runtime ownership check and interlock. It must
	// be fully configured.
	Store store.Store
	// Effector must enforce the final dispatch authorization check at the
	// effect boundary; an effector that cannot is refused at construction.
	Effector actionport.AuthorizedEffector
	Clock    clock.Clock
	IDs      ids.Generator
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
	clk      clock.Clock
	ids      ids.Generator
	owner    string
	leaseFor time.Duration
	observer LeaseObserver
}

// New validates the safety dependencies and applies clock, identity and lease
// defaults, none of which bypass authorization.
func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() {
		return nil, errors.New("actions require a database, runtime owner check and interlock")
	}
	if cfg.Effector == nil {
		return nil, errors.New("actions require an effector")
	}
	return &Service{store: cfg.Store, effector: cfg.Effector, clk: orPhysical(cfg.Clock), ids: orRandom(cfg.IDs),
		owner: orDefault(cfg.LeaseOwner, "actions"), leaseFor: orMinute(cfg.LeaseFor), observer: cfg.Observer}, nil
}

func orPhysical(clk clock.Clock) clock.Clock {
	if clk == nil {
		return clock.Physical()
	}
	return clk
}

func orRandom(generator ids.Generator) ids.Generator {
	if generator == nil {
		return ids.Random()
	}
	return generator
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
