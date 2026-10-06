package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// applyCounterfactual sends each distinct, complete counterfactual command to
// the simulator only. Replay never reaches an effector.
func applyCounterfactual(ctx context.Context, caps domain.Capabilities, result *domain.Result) error {
	if len(caps.Commands) == 0 {
		return fmt.Errorf("counterfactual command set is empty")
	}
	return simulateCommands(ctx, caps, result)
}

func simulateCommands(ctx context.Context, caps domain.Capabilities, result *domain.Result) error {
	seenCommands := make(map[string]struct{}, len(caps.Commands))
	for _, command := range caps.Commands {
		if err := domain.AdmitSimulatedCommand(command, seenCommands); err != nil {
			return err
		}
		if err := simulateCommand(ctx, caps.Simulator, command, result); err != nil {
			return err
		}
	}
	return nil
}

func simulateCommand(ctx context.Context, simulator domain.Simulator, command domain.SimulatedCommand, result *domain.Result) error {
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
