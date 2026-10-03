package replay

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestShadowManifestAdmissionPrecedesDecisionAndSnapshot(t *testing.T) {
	_, err := validateShadowOutput(ShadowInput{SnapshotJSON: []byte(`broken`)}, ShadowOutput{ExecutorVersion: "test", ManifestSHA256: "invalid", DecisionJSON: []byte(`broken`)}, nil, nil, "", time.Time{})
	if err == nil || !strings.HasPrefix(err.Error(), "manifest digest:") {
		t.Fatalf("expected manifest rejection first, got %v", err)
	}
}

func TestRecordedAttemptIdentityPrecedesFence(t *testing.T) {
	entry := RecordedEntry{EpisodeKey: "episode", EpisodeID: "ep", AttemptID: "attempt", Fence: 2}
	err := validateRecordedAttempt(entry, map[string]any{"episode_id": "ep", "attempt_id": "wrong", "fence": float64(3)})
	if err == nil || !strings.Contains(err.Error(), "mismatched attempt identity") {
		t.Fatalf("expected attempt identity rejection first, got %v", err)
	}
}
