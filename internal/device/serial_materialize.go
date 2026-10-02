package device

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// serial_materialize.go — the deterministic intent→device-command materializer
// for the physical (serial) effector boundary (Real-World Sensor HIL-0, Phase 03
// Task 3.2). It is the guarantee that no arbitrary target, pin, opcode, or PWM
// value ever comes from model output: a policy-approved actionport.Command carries
// only a bounded selector (an enum the model was allowed to write), and the
// concrete device parameters come from a CLOSED capability catalog that is
// configuration, never model output. The hard bounds are re-enforced here — at
// the last boundary — even though policy presets already produced bounded values
// (defense in depth). See docs/plans/real-world-sensor-hil/03-serial-effector.md.

// Materialize converts a policy-approved actionport.Command into a bounded device
// wire command (contractsv1.SchemaDeviceCommand). expectedBootID binds the
// command to the live device session. A route outside the catalog, a selector
// outside the preset set, or a parameter beyond its hard bound is rejected with
// zero commands emitted — never clamped silently.
func (c *CapabilityCatalog) Materialize(command actionport.Command, expectedBootID string) (map[string]any, error) {
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
	if err := spec.checkTarget(command); err != nil {
		return nil, err
	}
	parameters, err := spec.boundedParameters(command)
	if err != nil {
		return nil, err
	}
	physicalTarget := spec.Target

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

// checkTarget requires the command's normalized target to be the route's
// catalog target or one of its declared bindings.
func (spec OperationSpec) checkTarget(command actionport.Command) error {
	if command.NormalizedTarget == "" {
		return fmt.Errorf("normalized target is required to materialize a device command")
	}
	if command.NormalizedTarget == spec.Target {
		return nil
	}
	if boundTarget, ok := spec.TargetBindings[command.NormalizedTarget]; !ok || boundTarget != spec.Target {
		return fmt.Errorf("route %q target %q is not bound to catalog target %q", command.EffectorRoute, command.NormalizedTarget, spec.Target)
	}
	return nil
}

// boundedParameters copies the selected preset (the ONLY source of device
// parameters) and re-enforces every hard bound at this last boundary.
func (spec OperationSpec) boundedParameters(command actionport.Command) (map[string]any, error) {
	selectorValue, ok := command.Payload[spec.SelectorField].(string)
	if !ok || selectorValue == "" {
		return nil, fmt.Errorf("route %q requires a string selector %q in the command payload", command.EffectorRoute, spec.SelectorField)
	}
	preset, ok := spec.Presets[selectorValue]
	if !ok {
		return nil, fmt.Errorf("route %q selector %q=%q is not an allowed preset", command.EffectorRoute, spec.SelectorField, selectorValue)
	}
	parameters := make(map[string]any, len(preset))
	for key, value := range preset {
		parameters[key] = value
	}
	for _, param := range sortedKeys(spec.Bounds) {
		raw, present := parameters[param]
		if !present {
			continue
		}
		if err := spec.Bounds[param].check(command.EffectorRoute, param, raw); err != nil {
			return nil, err
		}
	}
	return parameters, nil
}

func (bound NumericBound) check(route, param string, raw any) error {
	number, ok := toFloat(raw)
	if !ok {
		return fmt.Errorf("route %q parameter %q is not numeric and cannot be bounded", route, param)
	}
	if bound.Max != nil && number > *bound.Max {
		return fmt.Errorf("route %q parameter %q=%v exceeds hard max %v", route, param, number, *bound.Max)
	}
	if bound.Min != nil && number < *bound.Min {
		return fmt.Errorf("route %q parameter %q=%v below hard min %v", route, param, number, *bound.Min)
	}
	return nil
}

// MaterializeSafeStop creates the fixed catalog-owned safe-state command for a
// target. It has no caller-supplied parameters or policy authority and cannot
// be used to clear a physical e-stop.
func (c *CapabilityCatalog) MaterializeSafeStop(target, expectedBootID string) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("capability catalog is required to materialize a safe stop")
	}
	if expectedBootID == "" {
		return nil, fmt.Errorf("expected boot id is required to materialize a safe stop")
	}
	spec, ok := c.SafeStops[target]
	if !ok {
		return nil, fmt.Errorf("safe stop target %q is not in the capability catalog", target)
	}
	catalogDigest, err := c.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest safe stop catalog: %w", err)
	}
	parameters := map[string]any{}
	commandID := "safe-stop/" + target
	document := map[string]any{
		"message_type": "command", "protocol_version": c.ProtocolVersion,
		"command_id": commandID, "target": target, "operation": spec.Operation,
		"parameters": parameters, "expected_boot_id": expectedBootID,
		"not_before_mono_us": 0, "expires_after_ms": spec.ExpiresAfterMs,
		"policy_digest": catalogDigest,
	}
	identity := cloneDocument(document)
	delete(identity, "command_id")
	idempotencyKey, err := canonicaljson.Digest(canonicaljson.DomainCommand, identity)
	if err != nil {
		return nil, fmt.Errorf("digest safe stop command identity: %w", err)
	}
	document["idempotency_key"] = idempotencyKey
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
		return nil, fmt.Errorf("materialized safe stop is invalid: %w", err)
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
