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
	fallback actionport.RoutedEffector
}

func NewCompositeEffector(watch *watch.Service, fallback actionport.RoutedEffector) *CompositeEffector {
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

func (e *CompositeEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	effector, err := e.route(command.EffectorRoute)
	if err != nil {
		return actionport.Effect{}, err
	}
	effect, err := effector.DispatchAuthorized(ctx, command, authorization)
	if err != nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorized %s effect: %w", command.EffectorRoute, err)
	}
	return effect, nil
}

func (e *CompositeEffector) route(route string) (actionport.AuthorizedEffector, error) {
	if e == nil {
		return nil, fmt.Errorf("composite effector is not configured")
	}
	owner := e.ownerOf(route)
	if owner == nil {
		return nil, unownedRouteError(route, e.unownedReason(route))
	}
	return owner, nil
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
