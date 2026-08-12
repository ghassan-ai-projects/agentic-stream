package actions

import (
	"context"
	"fmt"
)

// CompositeEffector routes internal watch installs to the durable watch
// effector and all other routes to the configured external/simulated effector.
// It keeps the watch implementation independent of the spec store.
type CompositeEffector struct {
	watch    *WatchEffector
	fallback Effector
}

// NewCompositeEffector creates the production action-plane composition.
func NewCompositeEffector(watch *WatchEffector, fallback Effector) *CompositeEffector {
	return &CompositeEffector{watch: watch, fallback: fallback}
}

// Dispatch routes one command without bypassing idempotency at the selected effector.
func (e *CompositeEffector) Dispatch(ctx context.Context, command Command) (Effect, error) {
	if command.EffectorRoute == "install_watch_condition" {
		if e.watch == nil {
			return Effect{}, fmt.Errorf("watch effector is not configured")
		}
		return e.watch.Dispatch(ctx, command)
	}
	effect, err := e.fallback.Dispatch(ctx, command)
	if err != nil {
		return Effect{}, fmt.Errorf("dispatch fallback effect: %w", err)
	}
	return effect, nil
}

// DispatchAuthorized preserves the final authorization check for either route.
func (e *CompositeEffector) DispatchAuthorized(ctx context.Context, command Command, authorization Authorization) (Effect, error) {
	if authorization.Check == nil {
		return Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if command.EffectorRoute == "install_watch_condition" {
		if e.watch == nil {
			return Effect{}, fmt.Errorf("watch effector is not configured")
		}
		return e.watch.DispatchAuthorized(ctx, command, authorization)
	}
	if guarded, ok := e.fallback.(AuthorizedEffector); ok {
		effect, err := guarded.DispatchAuthorized(ctx, command, authorization)
		if err != nil {
			return Effect{}, fmt.Errorf("dispatch authorized fallback effect: %w", err)
		}
		return effect, nil
	}
	return Effect{}, fmt.Errorf("fallback effector must implement AuthorizedEffector")
}
