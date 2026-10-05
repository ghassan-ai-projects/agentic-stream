package domain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func baselineFixtureInput() ShadowInput {
	return ShadowInput{
		TenantID: "default", EpisodeKey: "episode-1", EpisodeID: "episode-1", SituationID: "situation-1",
		SituationVersion: 1, AttemptID: "attempt-1", Fence: 1,
		SnapshotDigest: "sha256:" + strings.Repeat("1", 64),
		SnapshotJSON:   []byte(`{"entity":{"id":"motor-1"},"phase":"warning"}`),
	}
}

func predictiveStyleIntents() []Intent {
	return []Intent{{
		Type: "create_maintenance_ticket", Risk: "R1",
		ParameterSchema: map[string]any{
			"properties": map[string]any{
				"entity_id": map[string]any{"type": "string"},
				"priority":  map[string]any{"type": "string", "enum": []any{"low", "high"}},
			},
			"required": []any{"entity_id", "priority"},
		},
	}}
}

func TestBaselinePolicyRequiresCatalog(t *testing.T) {
	if _, err := NewBaselinePolicy(nil); err == nil {
		t.Fatal("empty catalog accepted")
	}
	var unconfigured *BaselinePolicy
	if _, err := unconfigured.ExecuteBaseline(context.Background(), baselineFixtureInput()); err == nil {
		t.Fatal("unconfigured baseline accepted")
	}
}

func TestBaselinePolicyProducesCanonicalValidatedDecision(t *testing.T) {
	t.Parallel()
	policy, err := NewBaselinePolicy(predictiveStyleIntents())
	if err != nil {
		t.Fatal(err)
	}
	input := baselineFixtureInput()
	output, err := policy.ExecuteBaseline(context.Background(), input)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if output.ExecutorVersion != "deterministic-baseline-v1" || output.DecisionSHA256 == "" {
		t.Fatalf("baseline output incomplete: %+v", output)
	}
	canonical, err := canonicaljson.Marshal(json.RawMessage(output.DecisionJSON))
	if err != nil || string(canonical) != string(output.DecisionJSON) {
		t.Fatalf("baseline decision is not canonical: %v", err)
	}
	if !strings.Contains(string(output.DecisionJSON), "create_maintenance_ticket") {
		t.Fatalf("baseline selected no intent: %s", output.DecisionJSON)
	}
	if !strings.Contains(string(output.DecisionJSON), `"priority":"low"`) {
		t.Fatalf("undeclared-phase enum should select the first value: %s", output.DecisionJSON)
	}
}

func TestBaselinePolicyMapsPhaseToDeclaredStateAndModeEnums(t *testing.T) {
	t.Parallel()
	intents := []Intent{{
		Type: "set_cooling", Risk: "R1",
		ParameterSchema: map[string]any{
			"properties": map[string]any{
				"state": map[string]any{"type": "string", "enum": []any{"watch", "alert"}},
				"mode":  map[string]any{"type": "string", "enum": []any{"hold", "bounded_cooling"}},
			},
			"required": []any{"state", "mode"},
		},
	}}
	policy, err := NewBaselinePolicy(intents)
	if err != nil {
		t.Fatal(err)
	}
	for phase, want := range map[string][]string{
		"over_ceiling": {`"state":"alert"`, `"mode":"bounded_cooling"`},
		"cooling":      {`"state":"watch"`, `"mode":"hold"`},
		"unknown":      {`"state":"watch"`, `"mode":"hold"`},
	} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			input := baselineFixtureInput()
			input.SnapshotJSON = []byte(`{"entity":{"id":"motor-1"},"phase":"` + phase + `"}`)
			output, err := policy.ExecuteBaseline(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range want {
				if !strings.Contains(string(output.DecisionJSON), field) {
					t.Fatalf("phase %s produced wrong parameters: %s", phase, output.DecisionJSON)
				}
			}
		})
	}
}

func TestBaselinePolicyAbstainsWhenRequiredParameterIsUnavailable(t *testing.T) {
	t.Parallel()
	intents := predictiveStyleIntents()
	intents[0].ParameterSchema = map[string]any{
		"properties": map[string]any{"entity_id": map[string]any{"type": "string"}},
		"required":   []any{"entity_id", "missing_field"},
	}
	policy, err := NewBaselinePolicy(intents)
	if err != nil {
		t.Fatal(err)
	}
	output, err := policy.ExecuteBaseline(context.Background(), ShadowInput{SnapshotJSON: []byte(`{"entity":{"id":"motor-1"},"phase":"warning"}`)})
	if err != nil {
		t.Fatalf("abstention must not fail: %v", err)
	}
	if !strings.Contains(string(output.DecisionJSON), "need_more_evidence") {
		t.Fatalf("baseline did not abstain: %s", output.DecisionJSON)
	}
}

func TestHashVersionDigestsIsOrderSensitive(t *testing.T) {
	t.Parallel()
	first := []VersionDigest{{SituationID: "a", Version: 1, SHA256: []byte{1}}, {SituationID: "a", Version: 2, SHA256: []byte{2}}}
	if HashVersionDigests(first) != HashVersionDigests(cloneVersions(first)) {
		t.Fatal("hash is not deterministic")
	}
	reordered := []VersionDigest{first[1], first[0]}
	if HashVersionDigests(first) == HashVersionDigests(reordered) {
		t.Fatal("hash ignored version order")
	}
	if HashVersionDigests(nil) == "" {
		t.Fatal("empty hash should still be defined")
	}
}

func cloneVersions(versions []VersionDigest) []VersionDigest {
	return append([]VersionDigest(nil), versions...)
}
