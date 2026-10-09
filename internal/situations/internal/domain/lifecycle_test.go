package domain_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type step struct {
	at         time.Duration
	value      float64
	wantPhase  string
	wantNoEdit bool
}

func runSteps(t *testing.T, engine *domain.Engine, steps []step) {
	t.Helper()
	for i, s := range steps {
		versions := apply(t, engine, vibration(s.value, base.Add(s.at)))
		if s.wantNoEdit {
			if len(versions) != 0 {
				t.Fatalf("step %d (value %v at %s) published %+v, want nothing", i, s.value, s.at, versions)
			}
			continue
		}
		if len(versions) != 1 || versions[0].Phase != s.wantPhase {
			t.Fatalf("step %d (value %v at %s) = %+v, want one version in phase %s", i, s.value, s.at, versions, s.wantPhase)
		}
	}
}

func TestFeaturesBelowTheOpenConditionPublishNothing(t *testing.T) {
	t.Parallel()
	runSteps(t, newEngine(t, vibrationSpec()), []step{
		{at: 0, value: 4.0, wantNoEdit: true},
		{at: time.Minute, value: 4.5, wantNoEdit: true},
	})
}

func TestSituationClimbsPhasesAsItsConditionsHold(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	runSteps(t, engine, []step{
		{at: 0, value: 5.0, wantPhase: "watch"},
		{at: time.Minute, value: 6.0, wantNoEdit: true},
		{at: 3 * time.Minute, value: 6.0, wantPhase: "warning"},
	})
	state, _, _, found, err := engine.CurrentState(0, "motor", "motor-17")
	if err != nil || !found || state.Version != 2 || state.Severity != 60 || state.PreviousPhase != "watch" {
		t.Fatalf("state = %+v found %v err %v, want version 2 in warning (severity 60) after watch", state, found, err)
	}
}

func TestTransitionWaitsForItsMinimumDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		steps []step
	}{
		{"condition held for the full duration", []step{
			{at: 0, value: 5.0, wantPhase: "watch"},
			{at: time.Minute, value: 6.0, wantNoEdit: true},
			{at: 3 * time.Minute, value: 6.0, wantPhase: "warning"},
		}},
		{"one nanosecond short of the duration", []step{
			{at: 0, value: 5.0, wantPhase: "watch"},
			{at: time.Minute, value: 6.0, wantNoEdit: true},
			{at: 3*time.Minute - time.Nanosecond, value: 6.0, wantNoEdit: true},
		}},
		{"condition lapsing restarts the clock", []step{
			{at: 0, value: 5.0, wantPhase: "watch"},
			{at: time.Minute, value: 6.0, wantNoEdit: true},
			{at: 2 * time.Minute, value: 5.0, wantNoEdit: true},
			{at: 3 * time.Minute, value: 6.0, wantNoEdit: true},
			{at: 4 * time.Minute, value: 6.0, wantNoEdit: true},
			{at: 5 * time.Minute, value: 6.0, wantPhase: "warning"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runSteps(t, newEngine(t, vibrationSpec()), tc.steps)
		})
	}
}

func TestConditionStartRecordsTheInstantAPendingTransitionFirstHeld(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(5.0, base))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if len(state.ConditionStart) != 0 {
		t.Fatalf("condition start after entering watch = %v, want it cleared by the phase change", state.ConditionStart)
	}
	apply(t, engine, vibration(6.0, base.Add(time.Minute)))
	state, _, _, _, _ = engine.CurrentState(0, "motor", "motor-17")
	if got := state.ConditionStart["watch->warning"]; !got.Equal(base.Add(time.Minute)) {
		t.Fatalf("condition start = %v, want the first instant the condition held (%s)", state.ConditionStart, base.Add(time.Minute))
	}
}

func TestOccurrenceResolvesWhenItsCloseConditionHolds(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	runSteps(t, engine, []step{
		{at: 0, value: 5.0, wantPhase: "watch"},
		{at: time.Minute, value: 2.0, wantPhase: domain.PhaseResolved},
		{at: 2 * time.Minute, value: 2.0, wantNoEdit: true},
		{at: 3 * time.Minute, value: 6.0, wantNoEdit: true},
	})
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if state.Phase != domain.PhaseResolved || state.PreviousPhase != "watch" || state.Severity != 0 || state.Version != 2 {
		t.Fatalf("state = %+v, want resolved after watch at severity 0, version 2", state)
	}
}

func TestEachPublishedVersionNamesItsPredecessorAndPhase(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	first := apply(t, engine, vibration(5.0, base))[0]
	apply(t, engine, vibration(6.0, base.Add(time.Minute)))
	second := apply(t, engine, vibration(6.0, base.Add(3*time.Minute)))[0]
	if first.Version != 1 || first.PreviousVersion != 0 || first.Phase != "watch" || first.PreviousPhase != "candidate" || first.Severity != 30 {
		t.Fatalf("first = %+v, want version 1 watch (severity 30) after candidate", first)
	}
	if second.Version != 2 || second.PreviousVersion != 1 || second.Phase != "warning" || second.PreviousPhase != "watch" || second.Severity != 60 {
		t.Fatalf("second = %+v, want version 2 warning (severity 60) after watch", second)
	}
	if first.SituationID != second.SituationID || first.OccurrenceID != second.OccurrenceID || first.EntityID != "motor-17" || first.Type != "bearing_degradation" {
		t.Fatalf("identity changed between versions: %+v then %+v", first, second)
	}
}

func TestSituationIdentityIsStableForTheSameDeploymentTenantPartitionAndEntity(t *testing.T) {
	t.Parallel()
	first := apply(t, newEngine(t, vibrationSpec()), vibration(5.0, base))[0]
	again := apply(t, newEngine(t, vibrationSpec()), vibration(5.0, base))[0]
	other := vibration(5.0, base)
	other.EntityID = "motor-18"
	different := apply(t, newEngine(t, vibrationSpec()), other)[0]
	if first.SituationID != again.SituationID || first.OccurrenceID != again.OccurrenceID {
		t.Fatalf("identity differs between identical engines: %s/%s vs %s/%s", first.SituationID, first.OccurrenceID, again.SituationID, again.OccurrenceID)
	}
	if first.SituationID == different.SituationID {
		t.Fatalf("two entities share situation id %s", first.SituationID)
	}
}

func TestCompletenessChangeOfALiveSituationPublishesANewVersion(t *testing.T) {
	t.Parallel()
	compiled := &spec.CompiledSpec{
		Digest: zeroDigest, SchemaVersion: "agentic-stream/v1",
		Situation: spec.Situation{
			Type: "bearing_degradation", InitialPhase: "watch",
			Occurrence: spec.Occurrence{OpenWhen: "true", CloseWhen: "false"},
			Phases:     []spec.Phase{{Name: "watch", Severity: 30}},
			Reducers:   []spec.Reducer{{Field: "facts.heartbeat_missing_5m", Strategy: "latest_event_time", Input: "heartbeat_missing_5m"}},
		},
	}
	engine := newEngine(t, compiled)
	healthy := operators.Feature{
		OutputName: "heartbeat_missing_5m", EntityType: "motor", EntityID: "motor-17",
		Value: false, Completeness: string(operators.CompletenessOnTime),
		EventTime: base, Watermark: base, InputEventIDs: []string{"hb-1"},
	}
	versions := apply(t, engine, healthy)
	if len(versions) != 1 || versions[0].Completeness != string(operators.CompletenessOnTime) {
		t.Fatalf("healthy source = %+v, want one on_time version", versions)
	}
	detection := base.Add(5*time.Minute + 30*time.Second)
	missing := healthy
	missing.Value, missing.Completeness, missing.EventTime, missing.Watermark = true, string(operators.CompletenessUncertain), detection, detection
	versions = apply(t, engine, missing)
	if len(versions) != 1 || versions[0].Version != 2 || versions[0].Completeness != string(operators.CompletenessUncertain) {
		t.Fatalf("source-health change = %+v, want version 2 uncertain", versions)
	}
	if got := versions[0].Facts["facts.heartbeat_missing_5m"]; got != true {
		t.Fatalf("latest_event_time fact = %v, want true", got)
	}
	if again := apply(t, engine, missing); len(again) != 0 {
		t.Fatalf("an unchanged feature published %+v, want nothing", again)
	}
}

func TestCompletenessChangeBeforeTheOccurrenceOpensPublishesNothing(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	feature := vibration(4.0, base)
	feature.Completeness = string(operators.CompletenessOnTime)
	if versions := apply(t, engine, feature); len(versions) != 0 {
		t.Fatalf("published %+v for a Situation that never opened, want nothing", versions)
	}
}

func TestValuesBetweenTheOpenAndCloseThresholdsDoNotFlapThePhase(t *testing.T) {
	t.Parallel()
	runSteps(t, newEngine(t, vibrationSpec()), []step{
		{at: 0, value: 5.0, wantPhase: "watch"},
		{at: time.Minute, value: 4.0, wantNoEdit: true},
		{at: 2 * time.Minute, value: 3.5, wantNoEdit: true},
		{at: 3 * time.Minute, value: 4.4, wantNoEdit: true},
		{at: 4 * time.Minute, value: 2.9, wantPhase: domain.PhaseResolved},
	})
}

func TestChainedTransitionsOfOneFeaturePublishOnlyTheFinalVersion(t *testing.T) {
	t.Parallel()
	compiled := vibrationSpec()
	compiled.Situation.Transitions[1].MinDuration = "0s"
	engine := newEngine(t, compiled)
	versions := apply(t, engine, vibration(6.0, base))
	if len(versions) != 1 {
		t.Fatalf("published %d versions for one feature, want exactly 1", len(versions))
	}
	got := versions[0]
	if got.Version != 2 || got.PreviousVersion != 1 || got.Phase != "warning" || got.PreviousPhase != "watch" {
		t.Fatalf("version = %d (previous %d) %s after %s, want version 2 (previous 1) in warning after watch: the intermediate version 1 is counted but never published", got.Version, got.PreviousVersion, got.Phase, got.PreviousPhase)
	}
	next := apply(t, engine, vibration(2.0, base.Add(time.Minute)))
	if len(next) != 1 || next[0].Version != 3 || next[0].PreviousVersion != 2 {
		t.Fatalf("next = %+v, want version 3 following the published version 2", next)
	}
}
