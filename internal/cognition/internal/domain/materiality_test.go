package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// zone-thermal's declaration: a phase change or a large severity change is
// material; a completeness flip or new facts are not.
const thermalMaterialDelta = "delta.phase_changed || delta.severity_change >= 10"

func materialityRules(t *testing.T, triggers ...spec.Trigger) *Rules {
	t.Helper()
	compiled := &spec.CompiledSpec{
		Situation: spec.Situation{Phases: []spec.Phase{{Name: "cooling"}, {Name: "recovering"}, {Name: "done", Terminal: true}}},
		Cognition: spec.Cognition{Triggers: triggers},
	}
	rules, err := NewRules(compiled)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestMaterialityFollowsTheSpecsDeclaration(t *testing.T) {
	t.Parallel()
	declared := spec.Trigger{Name: "overtemp", When: "false", Score: "1.0", Threshold: 1, MaterialDelta: thermalMaterialDelta}
	// Stored versions carry no occurrence id, as cognition loads them.
	previous := situations.Version{SituationID: "s", Version: 7, Phase: "cooling", Severity: 55, Completeness: "provisional", Facts: map[string]any{"temp_mean": 33.0}}
	resolved := previous
	resolved.Phase = situations.PhaseResolved
	cases := []struct {
		name     string
		rules    *Rules
		previous *situations.Version
		change   func(*situations.Version)
		want     bool
	}{
		{"completeness flip and new facts are not material", materialityRules(t, declared), &previous, func(v *situations.Version) {
			v.Completeness, v.Facts = "on_time", map[string]any{"temp_mean": 33.1}
		}, false},
		{"a phase change is material even when the trigger does not fire", materialityRules(t, declared), &previous, func(v *situations.Version) { v.Phase = "recovering" }, true},
		{"a severity jump is material", materialityRules(t, declared), &previous, func(v *situations.Version) { v.Severity = 85 }, true},
		{"the closing phase is always material", materialityRules(t, declared), &previous, func(v *situations.Version) { v.Phase = situations.PhaseResolved }, true},
		{"a terminal phase is always material", materialityRules(t, declared), &previous, func(v *situations.Version) { v.Phase = "done" }, true},
		{"the version after the occurrence ended is material", materialityRules(t, declared), &resolved, func(*situations.Version) {}, true},
		{"a first version is material", materialityRules(t, declared), nil, func(*situations.Version) {}, true},
		{"a trigger without materialDelta keeps every version material", materialityRules(t, declared, spec.Trigger{Name: "plain", When: "false", Score: "1.0", Threshold: 1}), &previous, func(*situations.Version) {}, true},
		{"a spec without triggers keeps every version material", materialityRules(t), &previous, func(*situations.Version) {}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			current := previous
			current.Version, current.OccurrenceID = 8, "occ-1"
			tc.change(&current)
			if got := tc.rules.Material(current, tc.previous); got != tc.want {
				t.Fatalf("Material = %v, want %v", got, tc.want)
			}
		})
	}
}
