package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

var ErrRouteOutlivesDispatch = errors.New("a route's command may outlive the dispatch call")

type BoundedCommand struct {
	Route          string
	Target         string
	Operation      string
	Parameters     map[string]any
	ExpiresAfterMs int
}

func (c *CapabilityCatalog) Bound(command actionport.Command) (BoundedCommand, error) {
	spec, err := c.route(command.EffectorRoute)
	if err != nil {
		return BoundedCommand{}, err
	}
	if err := spec.checkTarget(command); err != nil {
		return BoundedCommand{}, err
	}
	parameters, err := spec.boundedParameters(command)
	if err != nil {
		return BoundedCommand{}, err
	}
	return BoundedCommand{Route: command.EffectorRoute, Target: spec.Target, Operation: spec.Operation,
		Parameters: parameters, ExpiresAfterMs: spec.ExpiresAfterMs}, nil
}

func (c *CapabilityCatalog) route(name string) (Route, error) {
	if c == nil {
		return Route{}, fmt.Errorf("capability catalog is required to bound a device command")
	}
	spec, ok := c.Routes[name]
	if !ok {
		return Route{}, fmt.Errorf("route %q is not in the device capability catalog", name)
	}
	return spec, nil
}

func (c *CapabilityCatalog) CheckExpiriesWithin(callTimeout time.Duration) error {
	for _, name := range slices.Sorted(maps.Keys(c.Routes)) {
		expiry := time.Duration(c.Routes[name].ExpiresAfterMs) * time.Millisecond
		if expiry >= callTimeout {
			return fmt.Errorf("%w: route %q expires after %s, a dispatch call ends after %s; raise the dispatch lease",
				ErrRouteOutlivesDispatch, name, expiry, callTimeout)
		}
	}
	return nil
}
