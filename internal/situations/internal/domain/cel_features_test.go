package domain_test

import (
	"slices"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func celSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		Operators: []spec.Operator{
			{Name: "level_latest", Kind: "aggregate", Output: "level"},
			{Name: "alive", Kind: "missing_heartbeat", Output: "heartbeat_missing"},
			{Name: "drift", Kind: "slope", Output: "drift"},
		},
		Situation: spec.Situation{Reducers: []spec.Reducer{
			{Field: "facts.level", Strategy: "latest_event_time", Input: "level"},
			{Field: "facts.drift", Strategy: "latest_event_time", Input: "drift"},
			{Field: "evidence", Strategy: "set_union", Input: "level"},
		}},
	}
}

func TestCELFeaturesDefaultsEveryOperatorOutputThatHasNoFact(t *testing.T) {
	t.Parallel()
	features := domain.CELFeatures(celSpec(), map[string]any{"facts.level": 7.5, "facts.drift": nil}, []string{"evt-1"})
	if features["level"] != 7.5 {
		t.Fatalf("level = %v, want the reduced fact 7.5", features["level"])
	}
	if features["drift"] != 0.0 || features["heartbeat_missing"] != false {
		t.Fatalf("defaults = drift %v heartbeat_missing %v, want 0 and false", features["drift"], features["heartbeat_missing"])
	}
	if got, _ := features["evidence"].([]string); !slices.Equal(got, []string{"evt-1"}) {
		t.Fatalf("evidence = %v, want [evt-1]", features["evidence"])
	}
}

func TestCELFeaturesOfAnEmptySituationHoldOnlyDefaults(t *testing.T) {
	t.Parallel()
	features := domain.CELFeatures(celSpec(), nil, nil)
	if features["level"] != 0.0 || features["drift"] != 0.0 || features["heartbeat_missing"] != false {
		t.Fatalf("features = %v, want every output defaulted", features)
	}
}
