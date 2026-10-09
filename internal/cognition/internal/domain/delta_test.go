package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestDeltaUsesPreviousFactsAndConditionDefaults(t *testing.T) {
	t.Parallel()
	trigger := spec.Trigger{Name: "change", When: "delta.facts_changed", Score: "5.0", Threshold: 5}
	rules, err := NewRules(&spec.CompiledSpec{Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}}})
	if err != nil {
		t.Fatal(err)
	}
	previous := situations.Version{Phase: "watch", Facts: map[string]any{"temperature": 10.0}}
	current := situations.Version{SituationID: "s", Version: 2, Phase: "watch", Facts: map[string]any{"temperature": 20.0}}
	eval, err := rules.Evaluate(EvaluationInput{Trigger: trigger, Current: current, Previous: &previous, Now: time.Now(), DeploymentID: "dep"})
	if err != nil || eval.Outcome != "admitted" {
		t.Fatalf("changed facts = %+v %v", eval, err)
	}
	eval, err = rules.Evaluate(EvaluationInput{Trigger: trigger, Current: current, Previous: &current, Now: time.Now(), DeploymentID: "dep"})
	if err != nil || eval.Outcome != "ignored" {
		t.Fatalf("unchanged facts = %+v %v", eval, err)
	}
}

func TestDeltaDescribesWhatChangedSinceTheLastReasonedVersion(t *testing.T) {
	t.Parallel()
	rules := materialityRules(t)
	previous := situations.Version{Phase: "cooling", Severity: 30, Completeness: "provisional", Facts: map[string]any{"t": 1.0}}
	tests := []struct {
		name         string
		change       func(*situations.Version)
		wantPhase    bool
		wantSeverity int
		wantComplete bool
		wantFacts    bool
		wantNovelty  float64
	}{
		{"nothing changed", func(*situations.Version) {}, false, 0, false, false, 0},
		{"phase changed", func(v *situations.Version) { v.Phase = "recovering" }, true, 0, false, false, 1},
		{"severity rose", func(v *situations.Version) { v.Severity = 55 }, false, 25, false, false, 0},
		{"severity fell", func(v *situations.Version) { v.Severity = 10 }, false, -20, false, false, 0},
		{"completeness changed", func(v *situations.Version) { v.Completeness = "on_time" }, false, 0, true, false, 0},
		{"facts changed", func(v *situations.Version) { v.Facts = map[string]any{"t": 2.0} }, false, 0, false, true, 1},
		{"fact added", func(v *situations.Version) { v.Facts = map[string]any{"t": 1.0, "u": 1.0} }, false, 0, false, true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			current := previous
			current.Facts = map[string]any{"t": 1.0}
			tc.change(&current)
			delta := rules.buildDelta(current, &previous)
			got := []any{delta[spec.DeltaKeys.PhaseChanged], delta[spec.DeltaKeys.SeverityChange], delta[spec.DeltaKeys.CompletenessChanged], delta[spec.DeltaKeys.FactsChanged], delta[spec.DeltaKeys.Novelty]}
			want := []any{tc.wantPhase, tc.wantSeverity, tc.wantComplete, tc.wantFacts, tc.wantNovelty}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("delta %v, want %v", got, want)
				}
			}
		})
	}
}

func TestDeltaOfAFirstVersionReportsEverythingAsNew(t *testing.T) {
	t.Parallel()
	delta := materialityRules(t).buildDelta(situations.Version{Phase: "cooling", Severity: 40, Facts: map[string]any{"t": 1.0}}, nil)
	for _, key := range []string{spec.DeltaKeys.PhaseChanged, spec.DeltaKeys.CompletenessChanged, spec.DeltaKeys.PrimaryHypothesisChanged, spec.DeltaKeys.FactsChanged} {
		if delta[key] != true {
			t.Errorf("first-version delta %s = %v, want true", key, delta[key])
		}
	}
	if delta[spec.DeltaKeys.SeverityChange] != 40 || delta[spec.DeltaKeys.Novelty] != 1.0 || len(delta[spec.DeltaKeys.Facts].(map[string]any)) != 0 {
		t.Fatalf("first-version delta = %v, want severity change 40, novelty 1 and no previous facts", delta)
	}
}

func TestDeltaTreatsAPreviousVersionWithoutFactsAsEmpty(t *testing.T) {
	t.Parallel()
	delta := materialityRules(t).buildDelta(situations.Version{Facts: map[string]any{"t": 1.0}}, &situations.Version{})
	if delta[spec.DeltaKeys.FactsChanged] != true || len(delta[spec.DeltaKeys.Facts].(map[string]any)) != 0 {
		t.Fatalf("delta = %v, want changed facts against an empty past", delta)
	}
}

func TestTriggerSeesUncertaintyAsTheComplementOfConfidence(t *testing.T) {
	t.Parallel()
	trigger := spec.Trigger{Name: "t", When: "situation.uncertainty > 0.7 && situation.entity.id == 'm1'", Score: "1.0", Threshold: 1}
	rules := newRules(t, trigger)
	for confidence, want := range map[float64]string{0.2: "admitted", 0.5: "ignored"} {
		eval := evaluate(t, rules, trigger, situations.Version{SituationID: "s", Version: 1, EntityID: "m1", Confidence: confidence})
		if eval.Outcome != want {
			t.Errorf("confidence %v -> %s, want %s", confidence, eval.Outcome, want)
		}
	}
}

func TestMapsEqualComparesKeysAndValuesDeeply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b map[string]any
		want bool
	}{
		{"both empty", map[string]any{}, nil, true},
		{"same", map[string]any{"a": 1.0, "b": []any{1.0}}, map[string]any{"a": 1.0, "b": []any{1.0}}, true},
		{"different value", map[string]any{"a": 1.0}, map[string]any{"a": 2.0}, false},
		{"different key", map[string]any{"a": 1.0}, map[string]any{"b": 1.0}, false},
		{"different size", map[string]any{"a": 1.0}, map[string]any{"a": 1.0, "b": 1.0}, false},
		{"different nested value", map[string]any{"b": []any{1.0}}, map[string]any{"b": []any{2.0}}, false},
	}
	for _, tc := range tests {
		if got := mapsEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: mapsEqual = %v, want %v", tc.name, got, tc.want)
		}
	}
}
