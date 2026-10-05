package app

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

// CompositeEffector routes internal watch installs to the durable watch
// effector and all other routes to the configured external/simulated effector.
// It keeps the watch implementation independent of the spec store.
type CompositeEffector struct {
	watch    *watch.Service
	serial   actionport.VerifiedEffector
	fallback actionport.Effector
}

// NewCompositeEffector creates the production action-plane composition.
func NewCompositeEffector(watch *watch.Service, fallback actionport.Effector) *CompositeEffector {
	return &CompositeEffector{watch: watch, fallback: fallback}
}

// WithSerial configures the closed thermal routes to use the governed serial
// effector. A nil serial effector leaves those routes unavailable rather than
// silently sending them through the fallback.
func (e *CompositeEffector) WithSerial(serial actionport.VerifiedEffector) *CompositeEffector {
	if e != nil {
		e.serial = serial
	}
	return e
}

// Dispatch routes one command without bypassing idempotency at the selected effector.
func (e *CompositeEffector) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	effector, err := e.route(command.EffectorRoute)
	if err != nil {
		return actionport.Effect{}, err
	}
	effect, err := effector.Dispatch(ctx, command)
	if err != nil {
		return actionport.Effect{}, fmt.Errorf("dispatch %s effect: %w", command.EffectorRoute, err)
	}
	return effect, nil
}

func (e *CompositeEffector) route(route string) (actionport.Effector, error) {
	if e == nil {
		return nil, fmt.Errorf("composite effector is not configured")
	}
	switch domain.ClassifyRoute(route) {
	case domain.WatchRoute:
		if e.watch == nil {
			return nil, fmt.Errorf("watch effector is not configured")
		}
		return e.watch, nil
	case domain.DeviceRoute:
		return e.serialRoute(route)
	default:
		return e.fallbackRoute(route)
	}
}

func (e *CompositeEffector) serialRoute(route string) (actionport.Effector, error) {
	if e.serial == nil {
		return nil, fmt.Errorf("serial effector is not configured for route %q", route)
	}
	return e.serial, nil
}

func (e *CompositeEffector) fallbackRoute(route string) (actionport.Effector, error) {
	if e.fallback == nil {
		return nil, fmt.Errorf("no effector is configured for route %q", route)
	}
	return e.fallback, nil
}

// DispatchAuthorized preserves the final authorization check for either route.
func (e *CompositeEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	effector, err := e.route(command.EffectorRoute)
	if err != nil {
		return actionport.Effect{}, err
	}
	return dispatchAuthorizedRoute(ctx, effector, command, authorization)
}

func dispatchAuthorizedRoute(ctx context.Context, effector actionport.Effector, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	guarded, ok := effector.(actionport.AuthorizedEffector)
	if !ok {
		return actionport.Effect{}, fmt.Errorf("effector for route %q must implement AuthorizedEffector", command.EffectorRoute)
	}
	effect, err := guarded.DispatchAuthorized(ctx, command, authorization)
	if err != nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorized %s effect: %w", command.EffectorRoute, err)
	}
	return effect, nil
}

// VerifyDeviceCommand routes the one post-dispatch state query to the serial
// effector. Non-device actions return an empty status and remain governed by
// their existing effect semantics.
func (e *CompositeEffector) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	if e == nil {
		return "", nil, nil
	}
	switch domain.ClassifyRoute(command.EffectorRoute) {
	case domain.DeviceRoute:
		if e.serial == nil {
			return "", nil, nil
		}
		return e.verifySerialCommand(ctx, command)
	default:
		return "", nil, nil
	}
}

func (e *CompositeEffector) verifySerialCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	status, evidence, err := e.serial.VerifyDeviceCommand(ctx, command)
	if err != nil {
		return status, evidence, fmt.Errorf("%w", err)
	}
	return status, evidence, nil
}
