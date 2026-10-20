package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

type DeviceEffector interface {
	actionport.VerifiedEffector
	actionport.RouteOwner
}

type CompositeEffector struct {
	watch    *watch.Service
	device   DeviceEffector
	fallback actionport.Effector
}

func NewCompositeEffector(watch *watch.Service, fallback actionport.Effector) *CompositeEffector {
	return &CompositeEffector{watch: watch, fallback: fallback}
}

func (e *CompositeEffector) WithDevice(device DeviceEffector) *CompositeEffector {
	if e != nil {
		e.device = device
	}
	return e
}

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
	if route == watch.InstallRoute {
		return e.watchRoute()
	}
	if e.deviceOwns(route) {
		return e.device, nil
	}
	return e.fallbackRoute(route)
}

func (e *CompositeEffector) watchRoute() (actionport.Effector, error) {
	if e.watch == nil {
		return nil, fmt.Errorf("watch effector is not configured")
	}
	return e.watch, nil
}

func (e *CompositeEffector) deviceOwns(route string) bool {
	return e.device != nil && route != watch.InstallRoute && e.device.OwnsRoute(route)
}

func (e *CompositeEffector) fallbackRoute(route string) (actionport.Effector, error) {
	if e.fallback == nil {
		return nil, fmt.Errorf("no effector is configured for route %q", route)
	}
	return e.fallback, nil
}

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

func (e *CompositeEffector) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	if e == nil || !e.deviceOwns(command.EffectorRoute) {
		return "", nil, nil
	}
	return e.verifyDeviceCommand(ctx, command)
}

func (e *CompositeEffector) verifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	status, evidence, err := e.device.VerifyDeviceCommand(ctx, command)
	if err != nil {
		return status, evidence, fmt.Errorf("%w", err)
	}
	return status, evidence, nil
}
