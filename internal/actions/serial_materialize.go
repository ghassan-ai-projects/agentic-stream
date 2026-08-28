package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// serial_materialize.go — the deterministic intent→device-command materializer
// for the physical (serial) effector boundary (Real-World Sensor HIL-0, Phase 03
// Task 3.2). It is the guarantee that no arbitrary target, pin, opcode, or PWM
// value ever comes from model output: a policy-approved actions.Command carries
// only a bounded selector (an enum the model was allowed to write), and the
// concrete device parameters come from a CLOSED capability catalog that is
// configuration, never model output. The hard bounds are re-enforced here — at
// the last boundary — even though policy presets already produced bounded values
// (defense in depth). See docs/plans/real-world-sensor-hil/03-serial-effector.md.

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
	Bounds         map[string]NumericBound   `json:"bounds"`
}

// CapabilityCatalog is the whole closed device-capability surface for one
// serial effector. It is loaded from configuration (never a Go literal); the
// concrete bench values live in a JSON file the hardware owner tunes.
type CapabilityCatalog struct {
	ProtocolVersion int                      `json:"protocol_version"`
	Routes          map[string]OperationSpec `json:"routes"`
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
	return canonicaljson.Digest(canonicaljson.DomainCapabilityCatalog, c)
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
		// Every bounded parameter must be produced by every preset, so a
		// selector can never silently bypass a declared hard bound.
		for param := range spec.Bounds {
			if param == "" {
				return fmt.Errorf("route %q contains an empty bounds parameter", route)
			}
			for presetName, preset := range spec.Presets {
				if _, ok := preset[param]; !ok {
					return fmt.Errorf("route %q preset %q does not produce bounded parameter %q", route, presetName, param)
				}
			}
		}
		for param, bound := range spec.Bounds {
			if bound.Min != nil && !isFinite(*bound.Min) {
				return fmt.Errorf("route %q bound %q has a non-finite minimum", route, param)
			}
			if bound.Max != nil && !isFinite(*bound.Max) {
				return fmt.Errorf("route %q bound %q has a non-finite maximum", route, param)
			}
			if bound.Min != nil && bound.Max != nil && *bound.Min > *bound.Max {
				return fmt.Errorf("route %q bound %q has minimum %v above maximum %v", route, param, *bound.Min, *bound.Max)
			}
		}
	}
	return nil
}

// Materialize converts a policy-approved actions.Command into a bounded device
// wire command (contractsv1.SchemaDeviceCommand). expectedBootID binds the
// command to the live device session. A route outside the catalog, a selector
// outside the preset set, or a parameter beyond its hard bound is rejected with
// zero commands emitted — never clamped silently.
func (c *CapabilityCatalog) Materialize(command Command, expectedBootID string) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("capability catalog is required to materialize a device command")
	}
	if c.ProtocolVersion != contractsv1.DeviceProtocolVersion {
		return nil, fmt.Errorf("capability catalog protocol_version %d is unsupported, want %d", c.ProtocolVersion, contractsv1.DeviceProtocolVersion)
	}
	if command.CommandID == "" || command.IdempotencyKey == "" || command.PolicyDigest == "" {
		return nil, fmt.Errorf("command identity and policy digest are required to materialize a device command")
	}
	if expectedBootID == "" {
		return nil, fmt.Errorf("expected boot id is required to materialize a device command")
	}
	spec, ok := c.Routes[command.EffectorRoute]
	if !ok {
		return nil, fmt.Errorf("route %q is not in the device capability catalog", command.EffectorRoute)
	}
	if command.NormalizedTarget == "" {
		return nil, fmt.Errorf("normalized target is required to materialize a device command")
	}
	physicalTarget := spec.Target
	if command.NormalizedTarget != spec.Target {
		boundTarget, ok := spec.TargetBindings[command.NormalizedTarget]
		if !ok || boundTarget != spec.Target {
			return nil, fmt.Errorf("route %q target %q is not bound to catalog target %q", command.EffectorRoute, command.NormalizedTarget, spec.Target)
		}
	}
	selectorValue, ok := command.Payload[spec.SelectorField].(string)
	if !ok || selectorValue == "" {
		return nil, fmt.Errorf("route %q requires a string selector %q in the command payload", command.EffectorRoute, spec.SelectorField)
	}
	preset, ok := spec.Presets[selectorValue]
	if !ok {
		return nil, fmt.Errorf("route %q selector %q=%q is not an allowed preset", command.EffectorRoute, spec.SelectorField, selectorValue)
	}

	// Copy preset parameters (the ONLY source of device parameters) and
	// re-enforce every hard bound at this last boundary.
	parameters := make(map[string]any, len(preset))
	for key, value := range preset {
		parameters[key] = value
	}
	for _, param := range sortedKeys(spec.Bounds) {
		bound := spec.Bounds[param]
		raw, present := parameters[param]
		if !present {
			continue
		}
		number, ok := toFloat(raw)
		if !ok {
			return nil, fmt.Errorf("route %q parameter %q is not numeric and cannot be bounded", command.EffectorRoute, param)
		}
		if bound.Max != nil && number > *bound.Max {
			return nil, fmt.Errorf("route %q parameter %q=%v exceeds hard max %v", command.EffectorRoute, param, number, *bound.Max)
		}
		if bound.Min != nil && number < *bound.Min {
			return nil, fmt.Errorf("route %q parameter %q=%v below hard min %v", command.EffectorRoute, param, number, *bound.Min)
		}
	}

	document := map[string]any{
		"message_type":       "command",
		"protocol_version":   c.ProtocolVersion,
		"command_id":         command.CommandID,
		"idempotency_key":    command.IdempotencyKey,
		"target":             physicalTarget,
		"operation":          spec.Operation,
		"parameters":         parameters,
		"expected_boot_id":   expectedBootID,
		"not_before_mono_us": command.NotBeforeMonoUS,
		"expires_after_ms":   spec.ExpiresAfterMs,
		"policy_digest":      command.PolicyDigest,
	}
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
		return nil, fmt.Errorf("materialized device command is invalid: %w", err)
	}
	return document, nil
}

func sortedKeys(m map[string]NumericBound) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, isFinite(number)
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil && isFinite(parsed)
	default:
		return 0, false
	}
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
