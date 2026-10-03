package replay

import (
	"context"
	"fmt"
)

// applyCounterfactual sends each distinct, complete counterfactual command to
// the simulator only. Replay never reaches an effector.
func applyCounterfactual(ctx context.Context, caps Capabilities, result *Result) error {
	if len(caps.Commands) == 0 {
		return fmt.Errorf("counterfactual command set is empty")
	}
	return simulateCommands(ctx, caps, result)
}

func simulateCommands(ctx context.Context, caps Capabilities, result *Result) error {
	seenCommands := make(map[string]struct{}, len(caps.Commands))
	for _, command := range caps.Commands {
		if err := admitSimulatedCommand(command, seenCommands); err != nil {
			return err
		}
		if err := simulateCommand(ctx, caps.Simulator, command, result); err != nil {
			return err
		}
	}
	return nil
}

func admitSimulatedCommand(command SimulatedCommand, seenCommands map[string]struct{}) error {
	if command.CommandID == "" || command.Route == "" || command.Target == "" {
		return fmt.Errorf("counterfactual command is incomplete")
	}
	if _, exists := seenCommands[command.CommandID]; exists {
		return fmt.Errorf("counterfactual command %q is duplicated", command.CommandID)
	}
	seenCommands[command.CommandID] = struct{}{}
	return nil
}

func simulateCommand(ctx context.Context, simulator Simulator, command SimulatedCommand, result *Result) error {
	output, err := simulator.Simulate(ctx, command)
	if err != nil {
		return fmt.Errorf("simulate command %s: %w", command.CommandID, err)
	}
	if output == nil {
		return fmt.Errorf("simulator returned no outcome for command %s", command.CommandID)
	}
	result.SimulatedResults = append(result.SimulatedResults, output)
	result.CapabilityCalls++
	return nil
}
