package app

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func (e *CompositeEffector) RequireOwners(routes []string) error {
	if e == nil {
		return fmt.Errorf("composite effector is not configured")
	}
	for _, route := range routes {
		if !e.owns(route) {
			return fmt.Errorf("route %q has no effector: %s", route, e.unownedReason(route))
		}
	}
	return nil
}

func (e *CompositeEffector) owns(route string) bool {
	if route == watch.InstallRoute {
		return e.watch != nil
	}
	if e.device != nil {
		return e.device.OwnsRoute(route)
	}
	return fallbackOwns(e.fallback, route)
}

func fallbackOwns(fallback actionport.Effector, route string) bool {
	if fallback == nil {
		return false
	}
	owner, declaresRoutes := fallback.(actionport.RouteOwner)
	return !declaresRoutes || owner.OwnsRoute(route)
}

func (e *CompositeEffector) unownedReason(route string) string {
	switch {
	case route == watch.InstallRoute:
		return "the watch effector is not configured"
	case e.device != nil:
		return "it is not the watch route and the device catalog does not name it; with a device present the fallback owns no route"
	default:
		return "it is not the watch route and the fallback effector refuses it"
	}
}
