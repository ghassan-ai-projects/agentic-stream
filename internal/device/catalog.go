package device

import "github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

// CapabilityCatalog is the closed set of routes and safe stops a device
// accepts. It is configuration, never model output.
type CapabilityCatalog = domain.CapabilityCatalog

// LoadCapabilityCatalog parses and validates a capability catalog.
func LoadCapabilityCatalog(data []byte) (*CapabilityCatalog, error) {
	return domain.LoadCapabilityCatalog(data)
}
