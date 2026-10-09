package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestARestoredSituationKeepsItsResolutionAndInputCompleteness(t *testing.T) {
	t.Parallel()
	compiled := &spec.CompiledSpec{
		Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Situation: spec.Situation{
			Type: "bearing", InitialPhase: "candidate",
			Occurrence:  spec.Occurrence{OpenWhen: "features.level > 4.5", CloseWhen: "features.level < 3.0", ReopenCooldown: "5m"},
			Phases:      []spec.Phase{{Name: "candidate", Severity: 10}, {Name: situations.PhaseResolved}},
			Reducers:    []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
			Transitions: []spec.Transition{},
		},
	}
	engine, err := situations.NewEngine("d", "tenant", 0, compiled, sources.Deterministic())
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range []float64{5, 2} {
		at := base.Add(time.Duration(i) * time.Minute)
		feature := operators.Feature{OutputName: "level", EntityType: "motor", EntityID: "m1", Value: value, Completeness: string(operators.CompletenessOnTime), EventTime: at, Watermark: at}
		if _, err := engine.ApplyFeature(t.Context(), feature, at); err != nil {
			t.Fatal(err)
		}
	}
	current, stateJSON, digest, _, err := engine.CurrentState(0, "motor", "m1")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StoredSituation{SituationID: current.SituationID, Type: "bearing", EntityType: "motor", EntityID: "m1", OccurrenceID: current.OccurrenceID,
		Phase: current.Phase, Version: current.Version, StateCodecVersion: 1, StateJSON: stateJSON, StateSHA256: raw}.Restore("tenant", "d")
	if err != nil {
		t.Fatal(err)
	}
	if !restored.ResolvedAt.Equal(base.Add(time.Minute)) || restored.Inputs["level"] != operators.CompletenessOnTime {
		t.Fatalf("restored resolved at %s inputs %v, want resolution at 1m and level on_time", restored.ResolvedAt, restored.Inputs)
	}
}
