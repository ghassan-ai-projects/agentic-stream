package domain

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func (c *CapabilityCatalog) Materialize(command actionport.Command, expectedBootID string) (Command, error) {
	if err := c.checkMaterializable(command, expectedBootID); err != nil {
		return Command{}, err
	}
	bounded, err := c.Bound(command)
	if err != nil {
		return Command{}, err
	}
	return c.deviceCommandDocument(command, expectedBootID, bounded)
}

func (c *CapabilityCatalog) checkMaterializable(command actionport.Command, expectedBootID string) error {
	if c == nil {
		return fmt.Errorf("capability catalog is required to materialize a device command")
	}
	if c.ProtocolVersion != contractsv1.DeviceProtocolVersion {
		return fmt.Errorf("capability catalog protocol_version %d is unsupported, want %d", c.ProtocolVersion, contractsv1.DeviceProtocolVersion)
	}
	if command.CommandID == "" || command.IdempotencyKey == "" || command.PolicyDigest == "" {
		return fmt.Errorf("command identity and policy digest are required to materialize a device command")
	}
	if expectedBootID == "" {
		return fmt.Errorf("expected boot id is required to materialize a device command")
	}
	return nil
}

func (spec Route) resolveTarget(command actionport.Command) (string, error) {
	if command.NormalizedTarget == "" {
		return "", fmt.Errorf("normalized target is required to materialize a device command")
	}
	if spec.Target != "" && command.NormalizedTarget == spec.Target {
		return spec.Target, nil
	}
	boundTarget, ok := spec.TargetBindings[command.NormalizedTarget]
	if !ok || (spec.Target != "" && boundTarget != spec.Target) {
		return "", fmt.Errorf("route %q target %q is not bound to catalog target %v", command.EffectorRoute, command.NormalizedTarget, spec.Targets())
	}
	return boundTarget, nil
}

func (spec Route) boundedParameters(command actionport.Command, target string) (map[string]any, error) {
	parameters, err := spec.presetParameters(command)
	if err != nil {
		return nil, err
	}
	if spec.TargetArg != "" {
		parameters[spec.TargetArg] = target
	}
	return parameters, nil
}

func (spec Route) presetParameters(command actionport.Command) (map[string]any, error) {
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

func (c *CapabilityCatalog) deviceCommandDocument(command actionport.Command, expectedBootID string, bounded BoundedCommand) (Command, error) {
	return validatedCommand(Command{
		ProtocolVersion: c.ProtocolVersion, CommandID: command.CommandID, IdempotencyKey: command.IdempotencyKey,
		Target: bounded.Target, Operation: bounded.Operation, Parameters: bounded.Parameters, ExpectedBootID: expectedBootID,
		NotBeforeMonoUS: command.NotBeforeMonoUS, ExpiresAfterMs: bounded.ExpiresAfterMs, PolicyDigest: command.PolicyDigest,
	}, "materialized device command")
}

func validatedCommand(command Command, what string) (Command, error) {
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, command.Document()); err != nil {
		return Command{}, fmt.Errorf("%s is invalid: %w", what, err)
	}
	return command, nil
}

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
