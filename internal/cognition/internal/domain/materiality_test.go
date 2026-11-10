package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

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
	declared := spec.Trigger{Name: "overtemp", When: "true", Score: "1.0", Threshold: 1, MaterialDelta: thermalMaterialDelta}
	idle := spec.Trigger{Name: "idle", When: "false", Score: "1.0", Threshold: 1, MaterialDelta: thermalMaterialDelta}
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
		{"a phase change is material even when no trigger applies", materialityRules(t, idle), &previous, func(v *situations.Version) { v.Phase = "recovering" }, true},
		{"a severity jump is material to a trigger that applies", materialityRules(t, declared), &previous, func(v *situations.Version) { v.Severity = 85 }, true},
		{"a severity jump is not material to a trigger that does not apply", materialityRules(t, idle), &previous, func(v *situations.Version) { v.Severity = 85 }, false},
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

func TestAWatchFireIsMaterialOnlyWhileATriggerThatNamesItApplies(t *testing.T) {
	t.Parallel()
	rules := materialityRules(t,
		spec.Trigger{Name: "cooling_needs_relief", When: `situation.phase == "cooling"`, Score: "1.0", Threshold: 1, MaterialDelta: "delta.phase_changed || delta.watch_fired"},
		spec.Trigger{Name: "recovering_needs_its_step", When: `situation.phase == "recovering"`, Score: "1.0", Threshold: 1, MaterialDelta: "delta.phase_changed"})
	for _, tc := range []struct {
		phase string
		want  bool
	}{{"cooling", true}, {"recovering", false}} {
		t.Run(tc.phase, func(t *testing.T) {
			t.Parallel()
			previous := situations.Version{SituationID: "s", Version: 7, Phase: tc.phase, Severity: 55, Completeness: "final_by_policy", Facts: map[string]any{"temp_mean": 33.0}}
			woken := previous
			woken.Version, woken.WokenBy, woken.Facts = 8, situations.WokenByWatch, map[string]any{"temp_mean": 33.1}
			if got := rules.Material(woken, &previous); got != tc.want {
				t.Fatalf("a watch fire in phase %s: Material = %v, want %v", tc.phase, got, tc.want)
			}
		})
	}
}

func TestPrimaryHypothesisChangedIsTrueOnlyForTheFirstVersion(t *testing.T) {
	t.Parallel()
	rules := materialityRules(t)
	first := rules.buildDelta(situations.Version{Version: 1, Phase: "cooling"}, nil)
	later := rules.buildDelta(situations.Version{Version: 2, Phase: "recovering"}, &situations.Version{Version: 1, Phase: "cooling"})
	if first[spec.DeltaKeys.PrimaryHypothesisChanged] != true || later[spec.DeltaKeys.PrimaryHypothesisChanged] != false {
		t.Fatalf("first=%v later=%v", first[spec.DeltaKeys.PrimaryHypothesisChanged], later[spec.DeltaKeys.PrimaryHypothesisChanged])
	}
}
