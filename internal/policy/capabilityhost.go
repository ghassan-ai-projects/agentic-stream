package policy

import (
	"context"
	"fmt"
)

// Capability is a pure episode-facing tool. The host deliberately knows
// nothing about effectors, files, shells, deployment stores, or credentials.
type Capability struct {
	Name string
	Call func(context.Context, map[string]any) (map[string]any, error)
}

// CapabilityHost exposes only the explicitly supplied allowlist.
type CapabilityHost struct {
	capabilities map[string]Capability
}

// NewCapabilityHost creates an immutable allowlist host.
func NewCapabilityHost(allowlist []Capability) (*CapabilityHost, error) {
	host := &CapabilityHost{capabilities: make(map[string]Capability, len(allowlist))}
	for _, capability := range allowlist {
		if capability.Name == "" || capability.Call == nil {
			return nil, fmt.Errorf("capability name and call are required")
		}
		if _, exists := host.capabilities[capability.Name]; exists {
			return nil, fmt.Errorf("duplicate capability %q", capability.Name)
		}
		host.capabilities[capability.Name] = capability
	}
	return host, nil
}

// Call invokes one allowlisted capability and rejects everything else.
func (h *CapabilityHost) Call(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	if h == nil {
		return nil, fmt.Errorf("capability host is nil")
	}
	capability, ok := h.capabilities[name]
	if !ok {
		return nil, fmt.Errorf("capability %q is not allowlisted", name)
	}
	result, err := capability.Call(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("capability %s: %w", name, err)
	}
	return result, nil
}
