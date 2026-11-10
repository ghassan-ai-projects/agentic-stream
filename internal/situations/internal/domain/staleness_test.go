package domain_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func vibrationBesideALink(role string) *spec.CompiledSpec {
	compiled := vibrationSpec()
	compiled.Inputs = []spec.Input{{Name: "link", EventType: "test.heartbeat", CompletenessRole: role}}
	compiled.Operators = []spec.Operator{{Name: "link_missing", Kind: "missing_heartbeat", Inputs: []string{"link"}, Duration: "5m", Output: "link_missing_5m"}}
	compiled.Situation.Transitions[0].When = "features.vibration_rms > 4.5 && features.input_stale.link"
	return compiled
}

func linkLost(lost bool, at time.Time) operators.Feature {
	completeness := operators.CompletenessOnTime
	if lost {
		completeness = operators.CompletenessUncertain
	}
	return operators.Feature{OutputName: "link_missing_5m", EntityType: "motor", EntityID: "motor-17", Value: lost, EventTime: at, Watermark: at,
		Completeness: string(completeness)}
}

func settledVibration(value float64, at time.Time) operators.Feature {
	feature := vibration(value, at)
	feature.Completeness = string(operators.CompletenessFinalByPolicy)
	return feature
}

func TestTheLossOfAnAdvisoryInputIsAFactAndLeavesTheSituationsCompletenessAlone(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationBesideALink("advisory"))
	apply(t, engine, settledVibration(5.0, base))
	apply(t, engine, linkLost(false, base.Add(time.Minute)))
	versions := apply(t, engine, linkLost(true, base.Add(6*time.Minute)))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if len(versions) != 1 || versions[0].Phase != "watch" || state.Facts["input_stale.link"] != true {
		t.Fatalf("versions %+v, facts %v: want a version in watch, moved by features.input_stale.link, and the fact input_stale.link true", versions, state.Facts)
	}
	if state.Completeness != string(operators.CompletenessFinalByPolicy) {
		t.Fatalf("completeness = %q, want final_by_policy: a lost advisory input does not make the situation uncertain", state.Completeness)
	}
	back := apply(t, engine, linkLost(false, base.Add(7*time.Minute)))
	state, _, _, _, _ = engine.CurrentState(0, "motor", "motor-17")
	if len(back) != 1 || state.Facts["input_stale.link"] != false {
		t.Fatalf("versions %+v, facts %v: want a version when the input returns and the fact false", back, state.Facts)
	}
}

func TestTheLossOfARequiredInputMakesTheSituationUncertain(t *testing.T) {
	t.Parallel()
	compiled := vibrationBesideALink("required")
	compiled.Situation.Transitions[0].When = "features.vibration_rms > 4.5"
	engine := newEngine(t, compiled)
	apply(t, engine, settledVibration(5.0, base))
	apply(t, engine, linkLost(true, base.Add(6*time.Minute)))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if state.Completeness != string(operators.CompletenessUncertain) {
		t.Fatalf("completeness = %q, want uncertain", state.Completeness)
	}
	if _, stale := state.Facts["input_stale.link"]; stale {
		t.Fatalf("facts %v: a required input's loss is no staleness fact", state.Facts)
	}
}
