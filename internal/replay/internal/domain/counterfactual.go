package domain

import (
	"context"
	"fmt"
)

// SimulatedCommand is a typed counterfactual command. It is intentionally
// separate from the production action-plane command.
type SimulatedCommand struct {
	CommandID string
	Route     string
	Target    string
	Payload   map[string]any
}

// Simulator is the only capability accepted by counterfactual replay.
type Simulator interface {
	Simulate(context.Context, SimulatedCommand) (map[string]any, error)
}

// AdmitSimulatedCommand requires a complete, distinct command. A duplicate is
// rejected after its prior simulation was retained.
func AdmitSimulatedCommand(command SimulatedCommand, seenCommands map[string]struct{}) error {
	if command.CommandID == "" || command.Route == "" || command.Target == "" {
		return fmt.Errorf("counterfactual command is incomplete")
	}
	if _, exists := seenCommands[command.CommandID]; exists {
		return fmt.Errorf("counterfactual command %q is duplicated", command.CommandID)
	}
	seenCommands[command.CommandID] = struct{}{}
	return nil
}
