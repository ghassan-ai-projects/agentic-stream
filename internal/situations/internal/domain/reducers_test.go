package domain_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
)

func TestLatestEventTimeFactKeepsTheNewestObservation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		second    time.Duration
		wantFact  float64
		wantAfter float64
	}{
		{"an older observation arriving late", -time.Minute, 5.0, 5.0},
		{"an observation at the same instant", 0, 5.0, 5.0},
		{"a newer observation", time.Minute, 5.0, 8.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engine := newEngine(t, vibrationSpec())
			apply(t, engine, vibration(5.0, base.Add(10*time.Minute)))
			apply(t, engine, vibration(8.0, base.Add(10*time.Minute+tc.second)))
			state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
			if got := state.Facts["facts.vibration_rms"]; got != tc.wantAfter {
				t.Fatalf("fact = %v, want %v", got, tc.wantAfter)
			}
		})
	}
}

func TestSetUnionReducerAccumulatesEvidenceWithoutDuplicates(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(5.0, base, "evt-b", "evt-a"))
	versions := apply(t, engine, vibration(6.0, base.Add(3*time.Minute), "evt-c", "evt-a"))
	if len(versions) != 0 {
		t.Fatalf("unexpected version %+v", versions)
	}
	apply(t, engine, vibration(6.0, base.Add(6*time.Minute), "evt-d"))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if want := []string{"evt-b", "evt-c", "evt-a", "evt-d"}; !slices.Equal(state.Evidence, want) {
		t.Fatalf("evidence = %v, want %v, most recently seen last", state.Evidence, want)
	}
}

func TestSetUnionReducerKeepsOnlyTheMostRecentEvidenceWithinItsLimit(t *testing.T) {
	t.Parallel()
	compiled := vibrationSpec()
	compiled.Situation.Reducers[len(compiled.Situation.Reducers)-1].Limit = 2
	engine := newEngine(t, compiled)
	for i, id := range []string{"evt-1", "evt-2", "evt-3", "evt-2", "evt-4"} {
		apply(t, engine, vibration(5.0, base.Add(time.Duration(i)*time.Minute), id))
	}
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if want := []string{"evt-2", "evt-4"}; !slices.Equal(state.Evidence, want) {
		t.Fatalf("evidence = %v, want the two most recent %v", state.Evidence, want)
	}
}

func TestEvidenceWithoutADeclaredLimitStaysWithinTheSnapshotContract(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	ids := make([]string, domain.MaxEvidence+500)
	for i := range ids {
		ids[i] = fmt.Sprintf("evt-%05d", i)
	}
	apply(t, engine, vibration(5.0, base, ids...))
	versions := apply(t, engine, vibration(5.0, base.Add(time.Minute), "evt-last"))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if len(state.Evidence) != domain.MaxEvidence || state.Evidence[len(state.Evidence)-1] != "evt-last" {
		t.Fatalf("evidence holds %d references ending %q, want %d ending evt-last", len(state.Evidence), state.Evidence[len(state.Evidence)-1], domain.MaxEvidence)
	}
	for _, version := range versions {
		if len(version.Evidence) > domain.MaxEvidence {
			t.Fatalf("published %d evidence references, more than the snapshot allows", len(version.Evidence))
		}
	}
}

func TestPublishedEvidenceIsSortedAndFactsHideInternalBookkeeping(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	version := apply(t, engine, vibration(5.0, base, "evt-z", "evt-a", "evt-m"))[0]
	if !slices.Equal(version.Evidence, []string{"evt-a", "evt-m", "evt-z"}) {
		t.Fatalf("evidence = %v, want sorted ids", version.Evidence)
	}
	for key := range version.Facts {
		if len(key) > 11 && key[len(key)-11:] == "_event_time" {
			t.Fatalf("published facts expose bookkeeping key %q", key)
		}
	}
}

func TestFeatureWithoutAReducerLeavesNoFact(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(5.0, base))
	other := operators.Feature{OutputName: "temperature_mean", EntityType: "motor", EntityID: "motor-17", Value: 71.0, EventTime: base.Add(time.Minute), Watermark: base.Add(time.Minute)}
	apply(t, engine, other)
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if _, ok := state.Facts["facts.temperature_mean"]; ok || len(state.Facts) != 2 {
		t.Fatalf("facts = %v, want only the vibration fact and its event time", state.Facts)
	}
}

func TestTimerFeatureMetadataIsPublishedAsTimerProvenance(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	feature := vibration(5.0, base)
	feature.Metadata = map[string]any{"basis": "processing_time"}
	version := apply(t, engine, feature)[0]
	provenance, _ := version.Facts["timer_provenance"].(map[string]any)
	if provenance["basis"] != "processing_time" {
		t.Fatalf("facts = %v, want timer_provenance.basis processing_time", version.Facts)
	}
}
