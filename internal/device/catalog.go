package device

import "github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

// CapabilityCatalog is the closed set of routes and safe stops a device
// accepts. It is configuration, never model output.
type CapabilityCatalog = domain.CapabilityCatalog

// LoadCapabilityCatalog parses and validates a capability catalog.
func LoadCapabilityCatalog(data []byte) (*CapabilityCatalog, error) {
	return domain.LoadCapabilityCatalog(data)
}

// BoundedCommand is an approved command resolved by the catalog, independent
// of any device link: the route, its device target and operation, the preset's
// parameters after every hard bound, and the route's expiry. Each device link
// (device-wire, plant wire) builds its own record from it.
// CapabilityCatalog.Bound produces it; Materialize builds on it.
type BoundedCommand = domain.BoundedCommand

// ErrRouteOutlivesDispatch is returned by CapabilityCatalog.CheckExpiriesWithin
// for a route whose expires_after_ms does not end before a dispatch call does.
var ErrRouteOutlivesDispatch = domain.ErrRouteOutlivesDispatch
