package domain

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// The deterministic intent→device-command materializer
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
func (c *CapabilityCatalog) Materialize(command actionport.Command, expectedBootID string) (Command, error) {
	if c == nil {
		return Command{}, fmt.Errorf("capability catalog is required to materialize a device command")
	}
	if c.ProtocolVersion != contractsv1.DeviceProtocolVersion {
		return Command{}, fmt.Errorf("capability catalog protocol_version %d is unsupported, want %d", c.ProtocolVersion, contractsv1.DeviceProtocolVersion)
	}
	if command.CommandID == "" || command.IdempotencyKey == "" || command.PolicyDigest == "" {
		return Command{}, fmt.Errorf("command identity and policy digest are required to materialize a device command")
	}
	if expectedBootID == "" {
		return Command{}, fmt.Errorf("expected boot id is required to materialize a device command")
	}
	return c.materializeRoute(command, expectedBootID)
}

func (c *CapabilityCatalog) materializeRoute(command actionport.Command, expectedBootID string) (Command, error) {
	spec, ok := c.Routes[command.EffectorRoute]
	if !ok {
		return Command{}, fmt.Errorf("route %q is not in the device capability catalog", command.EffectorRoute)
	}
	if err := spec.checkTarget(command); err != nil {
		return Command{}, err
	}
	parameters, err := spec.boundedParameters(command)
	if err != nil {
		return Command{}, err
	}
	return c.deviceCommandDocument(command, expectedBootID, spec, parameters)
}

// checkTarget requires the command's normalized target to be the route's
// catalog target or one of its declared bindings.
func (spec Route) checkTarget(command actionport.Command) error {
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
func (spec Route) boundedParameters(command actionport.Command) (map[string]any, error) {
	selectorValue, ok := command.Payload[spec.SelectorField].(string)
	if !ok || selectorValue == "" {
		return nil, fmt.Errorf("route %q requires a string selector %q in the command payload", command.EffectorRoute, spec.SelectorField)
	}
	preset, ok := spec.Presets[selectorValue]
	if !ok {
		return nil, fmt.Errorf("route %q selector %q=%q is not an allowed preset", command.EffectorRoute, spec.SelectorField, selectorValue)
	}
	return spec.enforcePresetBounds(command.EffectorRoute, preset)
}

func (spec Route) enforcePresetBounds(route string, preset map[string]any) (map[string]any, error) {
	parameters := make(map[string]any, len(preset))
	for key, value := range preset {
		parameters[key] = value
	}
	for _, param := range sortedKeys(spec.Bounds) {
		raw, present := parameters[param]
		if !present {
			continue
		}
		if err := spec.Bounds[param].check(route, param, raw); err != nil {
			return nil, err
		}
	}
	return parameters, nil
}

func sortedKeys(m map[string]Bound) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (bound Bound) check(route, param string, raw any) error {
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

func (c *CapabilityCatalog) deviceCommandDocument(command actionport.Command, expectedBootID string, spec Route, parameters map[string]any) (Command, error) {
	return validatedCommand(Command{
		ProtocolVersion: c.ProtocolVersion, CommandID: command.CommandID, IdempotencyKey: command.IdempotencyKey,
		Target: spec.Target, Operation: spec.Operation, Parameters: parameters, ExpectedBootID: expectedBootID,
		NotBeforeMonoUS: command.NotBeforeMonoUS, ExpiresAfterMs: spec.ExpiresAfterMs, PolicyDigest: command.PolicyDigest,
	}, "materialized device command")
}

func validatedCommand(command Command, what string) (Command, error) {
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, command.Document()); err != nil {
		return Command{}, fmt.Errorf("%s is invalid: %w", what, err)
	}
	return command, nil
}

// MaterializeSafeStop creates the fixed catalog-owned safe-state command for a
// target. It has no caller-supplied parameters or policy authority and cannot
// be used to clear a physical e-stop.
func (c *CapabilityCatalog) MaterializeSafeStop(target, expectedBootID string) (Command, error) {
	if c == nil {
		return Command{}, fmt.Errorf("capability catalog is required to materialize a safe stop")
	}
	if expectedBootID == "" {
		return Command{}, fmt.Errorf("expected boot id is required to materialize a safe stop")
	}
	spec, ok := c.SafeStops[target]
	if !ok {
		return Command{}, fmt.Errorf("safe stop target %q is not in the capability catalog", target)
	}
	return c.safeStopCommand(target, expectedBootID, spec)
}

// safeStopCommand builds the safe stop and keys it by its own identity, so the
// same safe stop on the same boot is always the same command.
func (c *CapabilityCatalog) safeStopCommand(target, expectedBootID string, spec SafeStopRoute) (Command, error) {
	catalogDigest, err := c.Digest()
	if err != nil {
		return Command{}, fmt.Errorf("digest safe stop catalog: %w", err)
	}
	command := Command{
		ProtocolVersion: c.ProtocolVersion, CommandID: "safe-stop/" + target, Target: target, Operation: spec.Operation,
		Parameters: map[string]any{}, ExpectedBootID: expectedBootID, ExpiresAfterMs: spec.ExpiresAfterMs, PolicyDigest: catalogDigest,
	}
	if command.IdempotencyKey, err = command.Identity(); err != nil {
		return Command{}, fmt.Errorf("digest safe stop command identity: %w", err)
	}
	return validatedCommand(command, "materialized safe stop")
}
