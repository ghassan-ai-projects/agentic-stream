package app

import (
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// Config supplies the persistence store, which must be fully configured, and an
// optional clock.
type Config struct {
	Store store.Store
	Clock clock.Clock
}

// Service owns the configured watch use cases.
type Service struct {
	store store.Store
	clk   clock.Clock
}

// New requires a fully configured store and defaults the clock to the physical
// clock.
func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() {
		return nil, errors.New("watch requires a database, runtime owner check and interlock")
	}
	clk := cfg.Clock
	if clk == nil {
		clk = clock.Physical()
	}
	return &Service{store: cfg.Store, clk: clk}, nil
}
