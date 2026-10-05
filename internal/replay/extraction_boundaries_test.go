package replay

import (
	"context"
	"strings"
	"testing"
)

type countingSimulator struct{ calls int }

func (s *countingSimulator) Simulate(_ context.Context, command SimulatedCommand) (map[string]any, error) {
	s.calls++
	return map[string]any{"command_id": command.CommandID}, nil
}

func TestCounterfactualDuplicateRetainsPriorSimulation(t *testing.T) {
	simulator := &countingSimulator{}
	command := SimulatedCommand{CommandID: "one", Route: "simulator", Target: "motor"}
	result := &Result{}
	err := applyCounterfactual(t.Context(), Capabilities{Simulator: simulator, Commands: []SimulatedCommand{command, command}}, result)
	if err == nil || !strings.Contains(err.Error(), "is duplicated") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	if simulator.calls != 1 || result.CapabilityCalls != 1 || len(result.SimulatedResults) != 1 {
		t.Fatalf("prior simulation was not retained: calls=%d result=%+v", simulator.calls, result)
	}
}
