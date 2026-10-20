package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var updateBaselineChoices = flag.Bool("update", false, "rewrite the pinned baseline choices of the example specs")

const (
	exampleSpecsGlob     = "../../../../examples/*/*.situation.yaml"
	baselineChoicesGold  = "../../testdata/baseline/example-choices.json"
	baselineSnapshotUnit = "entity-1"
)

type baselineChoice struct {
	DecisionSHA256 string         `json:"decision_sha256"`
	IntentType     string         `json:"intent_type,omitempty"`
	Parameters     map[string]any `json:"parameters,omitempty"`
}

type exampleBaselineChoices struct {
	Catalog      map[string]baselineChoice            `json:"catalog"`
	SingleIntent map[string]map[string]baselineChoice `json:"single_intent"`
}

func TestBaselineMakesThePinnedChoicesForEveryExampleSpec(t *testing.T) {
	t.Parallel()
	got := make(map[string]exampleBaselineChoices)
	for _, path := range exampleSpecPaths(t) {
		got[exampleSpecName(path)] = choicesOfExampleSpec(t, path)
	}
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if *updateBaselineChoices {
		writeBaselineChoices(t, encoded)
		return
	}
	want, err := os.ReadFile(baselineChoicesGold)
	if err != nil {
		t.Fatalf("read pinned baseline choices: %v", err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("baseline choices of the example specs changed:\n%s\n(rerun with -update only for a reviewed behavior change)", encoded)
	}
}

func TestBaselineChoicesCoverEveryDeclaredPhaseAndIntent(t *testing.T) {
	t.Parallel()
	for _, path := range exampleSpecPaths(t) {
		compiled := compileExampleSpec(t, path)
		choices := choicesOfExampleSpec(t, path)
		if len(choices.Catalog) != len(compiled.Situation.Phases) || len(choices.Catalog) == 0 {
			t.Fatalf("%s: %d catalog choices for %d phases", path, len(choices.Catalog), len(compiled.Situation.Phases))
		}
		if len(choices.SingleIntent) != len(compiled.Actions.Intents) {
			t.Fatalf("%s: %d single-intent choices for %d intents", path, len(choices.SingleIntent), len(compiled.Actions.Intents))
		}
		for _, choice := range choices.Catalog {
			if choice.IntentType == "" || !strings.HasPrefix(choice.DecisionSHA256, "sha256:") {
				t.Fatalf("%s: the full catalog abstained or produced no decision digest: %+v", path, choice)
			}
		}
	}
}

func exampleSpecPaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(exampleSpecsGlob)
	if err != nil || len(paths) == 0 {
		t.Fatalf("find example specs: %v (%d found)", err, len(paths))
	}
	return paths
}

func exampleSpecName(path string) string {
	return filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)))
}

func compileExampleSpec(t *testing.T, path string) *spec.CompiledSpec {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), path)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return compiled
}

func choicesOfExampleSpec(t *testing.T, path string) exampleBaselineChoices {
	t.Helper()
	compiled := compileExampleSpec(t, path)
	catalog, err := NewDeterministicBaseline(compiled)
	if err != nil {
		t.Fatalf("baseline for %s: %v", path, err)
	}
	choices := exampleBaselineChoices{Catalog: choicesPerPhase(t, catalog, compiled), SingleIntent: make(map[string]map[string]baselineChoice)}
	intents := baselineIntents(compiled)
	for index, intent := range intents {
		alone, err := domain.NewBaselinePolicy(intents[index : index+1])
		if err != nil {
			t.Fatalf("baseline for %s intent %s: %v", path, intent.Type, err)
		}
		choices.SingleIntent[intent.Type] = choicesPerPhase(t, alone, compiled)
	}
	return choices
}

func choicesPerPhase(t *testing.T, policy *domain.BaselinePolicy, compiled *spec.CompiledSpec) map[string]baselineChoice {
	t.Helper()
	choices := make(map[string]baselineChoice, len(compiled.Situation.Phases))
	for _, phase := range compiled.Situation.Phases {
		output, err := policy.ExecuteBaseline(t.Context(), fixedBaselineSnapshot(phase.Name))
		if err != nil {
			t.Fatalf("baseline at phase %s: %v", phase.Name, err)
		}
		choices[phase.Name] = decodeBaselineChoice(t, output)
	}
	return choices
}

func fixedBaselineSnapshot(phase string) domain.ShadowInput {
	return domain.ShadowInput{
		TenantID: "default", EpisodeKey: "baseline-characterization", EpisodeID: "episode-1",
		SituationID: "situation-1", SituationVersion: 1, AttemptID: "attempt-1", Fence: 1,
		SnapshotDigest: "sha256:" + strings.Repeat("1", 64),
		SnapshotJSON:   []byte(`{"entity":{"id":"` + baselineSnapshotUnit + `"},"phase":"` + phase + `"}`),
	}
}

func decodeBaselineChoice(t *testing.T, output domain.ShadowOutput) baselineChoice {
	t.Helper()
	var decision struct {
		Intents []struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		} `json:"intents"`
	}
	if err := json.Unmarshal(output.DecisionJSON, &decision); err != nil {
		t.Fatalf("decode baseline decision: %v", err)
	}
	choice := baselineChoice{DecisionSHA256: output.DecisionSHA256}
	if len(decision.Intents) == 1 {
		choice.IntentType, choice.Parameters = decision.Intents[0].Type, decision.Intents[0].Parameters
	}
	return choice
}

func writeBaselineChoices(t *testing.T, encoded []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(baselineChoicesGold), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselineChoicesGold, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
