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
		if e.ownerOf(route) == nil {
			return unownedRouteError(route, e.unownedReason(route))
		}
	}
	return nil
}

func (e *CompositeEffector) OwnsRoute(route string) bool {
	return e != nil && e.ownerOf(route) != nil
}

func (e *CompositeEffector) ownerOf(route string) actionport.AuthorizedEffector {
	switch {
	case route == watch.InstallRoute:
		return e.watchOwner()
	case e.device != nil:
		return e.deviceOwner(route)
	default:
		return e.fallbackOwner(route)
	}
}

func (e *CompositeEffector) watchOwner() actionport.AuthorizedEffector {
	if e.watch == nil {
		return nil
	}
	return e.watch
}

func (e *CompositeEffector) deviceOwner(route string) actionport.AuthorizedEffector {
	if !e.deviceOwns(route) {
		return nil
	}
	return e.device
}

func (e *CompositeEffector) deviceOwns(route string) bool {
	return e.device != nil && route != watch.InstallRoute && e.device.OwnsRoute(route)
}

func (e *CompositeEffector) fallbackOwner(route string) actionport.AuthorizedEffector {
	if e.fallback == nil || !e.fallback.OwnsRoute(route) {
		return nil
	}
	return e.fallback
}

func unownedRouteError(route, reason string) error {
	return fmt.Errorf("route %q has no effector: %s", route, reason)
}

func (e *CompositeEffector) unownedReason(route string) string {
	switch {
	case route == watch.InstallRoute:
		return "the watch effector is not configured"
	case e.device != nil:
		return "it is not the watch route and the device catalog does not name it; with a device present the fallback owns no route"
	case e.fallback == nil:
		return "it is not the watch route and no fallback effector is configured"
	default:
		return "it is not the watch route and the fallback effector does not own it"
	}
}
