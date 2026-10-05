package domain

import (
	"fmt"
	"maps"
	"slices"
)

// leaseParameter is the command lifetime parameter; it is never an output
// value.
const leaseParameter = "lease_ms"

// OutputVerified reports whether the current output in a fresh device state is
// what the command asked for: same target and operation, energized exactly
// when the requested value is positive, and the same value. A fan is not
// verified merely because it is energized; a wrong duty fails like a wrong LED
// level. A command with more than one numeric output parameter cannot be
// verified unambiguously and is an error.
func OutputVerified(state, command map[string]any) (bool, error) {
	expectedValue, expectedEnergized, err := expectedOutput(command)
	if err != nil {
		return false, err
	}
	output, _ := state["current_output"].(map[string]any)
	observedEnergized, _ := output["energized"].(bool)
	observedValue, _ := output["value"].(float64)
	return output["target"] == command["target"] && output["operation"] == command["operation"] &&
		observedEnergized == expectedEnergized && observedValue == expectedValue, nil
}

// expectedOutput is the command's single numeric output parameter.
func expectedOutput(command map[string]any) (float64, bool, error) {
	parameters, _ := command["parameters"].(map[string]any)
	var outputs []float64
	for _, name := range slices.Sorted(maps.Keys(parameters)) {
		if value, ok := parameters[name].(float64); ok && name != leaseParameter {
			outputs = append(outputs, value)
		}
	}
	switch len(outputs) {
	case 0:
		return 0, false, nil
	case 1:
		return outputs[0], outputs[0] > 0, nil
	default:
		return 0, false, fmt.Errorf("command has %d numeric output parameters; verification needs exactly one", len(outputs))
	}
}
