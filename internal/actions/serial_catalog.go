package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"io"
	"math"
)

// NumericBound is an inclusive hard limit re-enforced on a materialized device
// parameter. An absent Min or Max means that side is unbounded.
type NumericBound struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// OperationSpec is the closed mapping from one accepted intent route to one
// bounded device operation. The model may only select a preset by name; it can
// never introduce a target, operation, or parameter value.
type OperationSpec struct {
	Operation      string                    `json:"operation"`
	Target         string                    `json:"target"`
	TargetBindings map[string]string         `json:"target_bindings,omitempty"`
	SelectorField  string                    `json:"selector_field"`
	ExpiresAfterMs int                       `json:"expires_after_ms"`
	Presets        map[string]map[string]any `json:"presets"`
	Bounds         map[string]NumericBound   `json:"bounds,omitempty"`
}

// SafeStopSpec is a catalog-owned, parameter-free operation that requests the
// device's safe state. It is never produced from model or intent payloads.
type SafeStopSpec struct {
	Operation      string `json:"operation"`
	ExpiresAfterMs int    `json:"expires_after_ms"`
}

// CapabilityCatalog is the whole closed device-capability surface for one
// serial effector. It is loaded from configuration (never a Go literal); the
// concrete bench values live in a JSON file the hardware owner tunes.
type CapabilityCatalog struct {
	ProtocolVersion int                      `json:"protocol_version"`
	Routes          map[string]OperationSpec `json:"routes"`
	SafeStops       map[string]SafeStopSpec  `json:"safe_stops,omitempty"`
}

// Digest returns the canonical identity of this validated capability catalog.
// A device session must match this digest before the catalog can authorize a
// command, preventing a stale local route table from being used with a new
// firmware capability set.
func (c *CapabilityCatalog) Digest() (string, error) {
	if c == nil {
		return "", fmt.Errorf("capability catalog is required")
	}
	if err := c.validate(); err != nil {
		return "", fmt.Errorf("validate capability catalog: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainCapabilityCatalog, c)
	if err != nil {
		return "", fmt.Errorf("digest capability catalog: %w", err)
	}
	return digest, nil
}

// LoadCapabilityCatalog parses and validates a capability catalog. A structurally
// invalid catalog is rejected — the effector must never load a catalog it cannot
// fully enforce.
func LoadCapabilityCatalog(data []byte) (*CapabilityCatalog, error) {
	var catalog CapabilityCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode capability catalog: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("capability catalog contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing capability catalog data: %w", err)
	}
	if err := catalog.validate(); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (c *CapabilityCatalog) validate() error {
	if c.ProtocolVersion != contractsv1.DeviceProtocolVersion {
		return fmt.Errorf("capability catalog protocol_version %d is unsupported, want %d", c.ProtocolVersion, contractsv1.DeviceProtocolVersion)
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("capability catalog has no routes")
	}
	for route, spec := range c.Routes {
		if err := spec.validate(route); err != nil {
			return err
		}
	}
	for target, spec := range c.SafeStops {
		if target == "" || spec.Operation != "safe_stop" {
			return fmt.Errorf("safe stop %q must use the catalog operation %q", target, "safe_stop")
		}
		if spec.ExpiresAfterMs < 1 || spec.ExpiresAfterMs > 86400000 {
			return fmt.Errorf("safe stop %q must set expires_after_ms between 1 and 86400000", target)
		}
	}
	return nil
}

func (spec OperationSpec) validate(route string) error {
	if route == "" {
		return fmt.Errorf("capability catalog contains an empty route")
	}
	if spec.Operation == "" || spec.Target == "" || spec.SelectorField == "" {
		return fmt.Errorf("route %q must set operation, target, and selector_field", route)
	}
	for logicalTarget, physicalTarget := range spec.TargetBindings {
		if logicalTarget == "" || physicalTarget == "" {
			return fmt.Errorf("route %q contains an empty target binding", route)
		}
		if physicalTarget != spec.Target {
			return fmt.Errorf("route %q target binding %q resolves to %q, want route target %q", route, logicalTarget, physicalTarget, spec.Target)
		}
	}
	if spec.ExpiresAfterMs < 1 {
		return fmt.Errorf("route %q must set a positive expires_after_ms", route)
	}
	if len(spec.Presets) == 0 {
		return fmt.Errorf("route %q has no presets", route)
	}
	return spec.validateBounds(route)
}

// validateBounds requires every bounded parameter to be produced by every
// preset, so a selector can never silently bypass a declared hard bound, and
// every bound to be finite and ordered.
func (spec OperationSpec) validateBounds(route string) error {
	for param := range spec.Bounds {
		if err := spec.requireBoundedByEveryPreset(route, param); err != nil {
			return err
		}
	}
	for param, bound := range spec.Bounds {
		if err := bound.validate(route, param); err != nil {
			return err
		}
	}
	return nil
}

func (spec OperationSpec) requireBoundedByEveryPreset(route, param string) error {
	if param == "" {
		return fmt.Errorf("route %q contains an empty bounds parameter", route)
	}
	for presetName, preset := range spec.Presets {
		if _, ok := preset[param]; !ok {
			return fmt.Errorf("route %q preset %q does not produce bounded parameter %q", route, presetName, param)
		}
	}
	return nil
}

// validate requires finite limits with the minimum at or below the maximum.
func (bound NumericBound) validate(route, param string) error {
	if bound.Min != nil && !isFinite(*bound.Min) {
		return fmt.Errorf("route %q bound %q has a non-finite minimum", route, param)
	}
	if bound.Max != nil && !isFinite(*bound.Max) {
		return fmt.Errorf("route %q bound %q has a non-finite maximum", route, param)
	}
	if bound.Min != nil && bound.Max != nil && *bound.Min > *bound.Max {
		return fmt.Errorf("route %q bound %q has minimum %v above maximum %v", route, param, *bound.Min, *bound.Max)
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
