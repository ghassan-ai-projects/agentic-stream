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
	serial   *SerialEffector
	fallback Effector
}

// NewCompositeEffector creates the production action-plane composition.
func NewCompositeEffector(watch *WatchEffector, fallback Effector) *CompositeEffector {
	return &CompositeEffector{watch: watch, fallback: fallback}
}

// WithSerial configures the closed thermal routes to use the governed serial
// effector. A nil serial effector leaves those routes unavailable rather than
// silently sending them through the fallback.
func (e *CompositeEffector) WithSerial(serial *SerialEffector) *CompositeEffector {
	if e != nil {
		e.serial = serial
	}
	return e
}

// Dispatch routes one command without bypassing idempotency at the selected effector.
func (e *CompositeEffector) Dispatch(ctx context.Context, command Command) (Effect, error) {
	effector, err := e.route(command.EffectorRoute)
	if err != nil {
		return Effect{}, err
	}
	effect, err := effector.Dispatch(ctx, command)
	if err != nil {
		return Effect{}, fmt.Errorf("dispatch %s effect: %w", command.EffectorRoute, err)
	}
	return effect, nil
}

// DispatchAuthorized preserves the final authorization check for either route.
func (e *CompositeEffector) DispatchAuthorized(ctx context.Context, command Command, authorization Authorization) (Effect, error) {
	if authorization.Check == nil {
		return Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	effector, err := e.route(command.EffectorRoute)
	if err != nil {
		return Effect{}, err
	}
	guarded, ok := effector.(AuthorizedEffector)
	if !ok {
		return Effect{}, fmt.Errorf("effector for route %q must implement AuthorizedEffector", command.EffectorRoute)
	}
	effect, err := guarded.DispatchAuthorized(ctx, command, authorization)
	if err != nil {
		return Effect{}, fmt.Errorf("dispatch authorized %s effect: %w", command.EffectorRoute, err)
	}
	return effect, nil
}

func (e *CompositeEffector) route(route string) (Effector, error) {
	if e == nil {
		return nil, fmt.Errorf("composite effector is not configured")
	}
	switch route {
	case "install_watch_condition":
		if e.watch == nil {
			return nil, fmt.Errorf("watch effector is not configured")
		}
		return e.watch, nil
	case "set_indicator", "select_thermal_mode":
		if e.serial == nil {
			return nil, fmt.Errorf("serial effector is not configured for route %q", route)
		}
		return e.serial, nil
	default:
		if e.fallback == nil {
			return nil, fmt.Errorf("no effector is configured for route %q", route)
		}
		return e.fallback, nil
	}
}
