package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type Bound struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

type Route struct {
	Operation      string                    `json:"operation"`
	Target         string                    `json:"target"`
	TargetBindings map[string]string         `json:"target_bindings,omitempty"`
	SelectorField  string                    `json:"selector_field"`
	ExpiresAfterMs int                       `json:"expires_after_ms"`
	Presets        map[string]map[string]any `json:"presets"`
	Bounds         map[string]Bound          `json:"bounds,omitempty"`
	TargetArg      string                    `json:"target_arg,omitempty"`
}

type SafeStopRoute struct {
	Operation      string `json:"operation"`
	ExpiresAfterMs int    `json:"expires_after_ms"`
}

type CapabilityCatalog struct {
	ProtocolVersion int                      `json:"protocol_version"`
	Routes          map[string]Route         `json:"routes"`
	SafeStops       map[string]SafeStopRoute `json:"safe_stops,omitempty"`
}

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

func LoadCapabilityCatalog(data []byte) (*CapabilityCatalog, error) {
	var catalog CapabilityCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode capability catalog: %w", err)
	}
	return finishCapabilityCatalog(decoder, &catalog)
}

func finishCapabilityCatalog(decoder *json.Decoder, catalog *CapabilityCatalog) (*CapabilityCatalog, error) {
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("capability catalog contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing capability catalog data: %w", err)
	}
	if err := catalog.validate(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (c *CapabilityCatalog) NamesRoute(route string) bool {
	if c == nil {
		return false
	}
	_, named := c.Routes[route]
	return named
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
	return c.validateSafeStops()
}

func (spec Route) validate(route string) error {
	if route == "" {
		return fmt.Errorf("capability catalog contains an empty route")
	}
	if route == actionport.WatchInstallRoute {
		return fmt.Errorf("route %q is reserved for the runtime's watch effector; a device catalog may not name it", route)
	}
	if spec.Operation == "" || spec.SelectorField == "" || len(spec.Targets()) == 0 {
		return fmt.Errorf("route %q must set operation, a target or target bindings, and selector_field", route)
	}
	if err := spec.validateTargetBindings(route); err != nil {
		return err
	}
	return spec.validatePresets(route)
}

func (spec Route) validateTargetBindings(route string) error {
	for logicalTarget, physicalTarget := range spec.TargetBindings {
		if logicalTarget == "" || physicalTarget == "" {
			return fmt.Errorf("route %q contains an empty target binding", route)
		}
		if spec.Target != "" && physicalTarget != spec.Target {
			return fmt.Errorf("route %q target binding %q resolves to %q, want route target %q", route, logicalTarget, physicalTarget, spec.Target)
		}
	}
	return nil
}

func (spec Route) Targets() []string {
	if spec.Target != "" {
		return []string{spec.Target}
	}
	targets := make([]string, 0, len(spec.TargetBindings))
	for _, logical := range slices.Sorted(maps.Keys(spec.TargetBindings)) {
		if physical := spec.TargetBindings[logical]; !slices.Contains(targets, physical) {
			targets = append(targets, physical)
		}
	}
	return targets
}

func (spec Route) validatePresets(route string) error {
	if spec.ExpiresAfterMs < 1 {
		return fmt.Errorf("route %q must set a positive expires_after_ms", route)
	}
	if len(spec.Presets) == 0 {
		return fmt.Errorf("route %q has no presets", route)
	}
	if err := spec.validateTargetArg(route); err != nil {
		return err
	}
	return spec.validateBounds(route)
}

func (spec Route) validateTargetArg(route string) error {
	if spec.TargetArg == "" {
		return nil
	}
	for presetName, preset := range spec.Presets {
		if _, ok := preset[spec.TargetArg]; ok {
			return fmt.Errorf("route %q preset %q sets %q, which the route fills with its target", route, presetName, spec.TargetArg)
		}
	}
	return nil
}

func (spec Route) validateBounds(route string) error {
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

func (spec Route) requireBoundedByEveryPreset(route, param string) error {
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

func (bound Bound) validate(route, param string) error {
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

func (c *CapabilityCatalog) validateSafeStops() error {
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
