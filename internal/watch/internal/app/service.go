package app

import (
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

type Config struct {
	Store    store.Store
	Clock    sources.Clock
	Features FeatureSource
}

type Service struct {
	store    store.Store
	clk      sources.Clock
	features FeatureSource
}

func New(cfg Config) (*Service, error) {
	if !cfg.Store.Configured() {
		return nil, errors.New("watch requires a database and runtime owner check")
	}
	clk := cfg.Clock
	clk = sources.OrPhysical(clk)
	return &Service{store: cfg.Store, clk: clk, features: cfg.Features}, nil
}
