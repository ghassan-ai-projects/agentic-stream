package actions

import (
	"context"
	"fmt"
	"sync"
)

// SimulatedEffector is the deterministic effector used by the first live
// vertical slice. It models a provider that accepts maintenance-ticket
// commands and remembers results by idempotency key.
type SimulatedEffector struct {
	mu      sync.Mutex
	results map[string]Effect
}

// NewSimulatedEffector creates an in-memory simulator effector.
func NewSimulatedEffector() *SimulatedEffector {
	return &SimulatedEffector{results: make(map[string]Effect)}
}

// Dispatch applies a supported simulated command exactly once per
// idempotency key. The returned result is stable across duplicate dispatches.
func (e *SimulatedEffector) Dispatch(ctx context.Context, command Command) (Effect, error) {
	if err := ctx.Err(); err != nil {
		return Effect{}, err
	}
	if command.EffectorRoute != "maintenance.ticket" && command.EffectorRoute != "sim.effector" {
		return Effect{}, fmt.Errorf("simulated effector does not support route %q", command.EffectorRoute)
	}
	if command.IdempotencyKey == "" {
		return Effect{}, fmt.Errorf("idempotency key is required")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if result, ok := e.results[command.IdempotencyKey]; ok {
		return result, nil
	}
	result := Effect{
		ProviderResult: map[string]any{
			"accepted":        true,
			"provider":        "agentic-stream-simulator",
			"idempotency_key": command.IdempotencyKey,
		},
		ObservedEffect: map[string]any{
			"route":  command.EffectorRoute,
			"target": command.NormalizedTarget,
		},
	}
	e.results[command.IdempotencyKey] = result
	return result, nil
}
