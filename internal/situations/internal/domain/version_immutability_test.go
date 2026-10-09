package domain_test

import (
	"bytes"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
)

func frozen(v domain.Version) domain.Version {
	v.Facts = maps.Clone(v.Facts)
	v.Evidence = slices.Clone(v.Evidence)
	v.ConditionStart = maps.Clone(v.ConditionStart)
	v.SnapshotJSON = bytes.Clone(v.SnapshotJSON)
	v.StateJSON = bytes.Clone(v.StateJSON)
	return v
}

func TestPublishedSituationVersionNeverChangesAfterLaterFeatures(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	published := apply(t, engine, vibration(5.0, base, "evt-1"))[0]
	want := frozen(published)
	apply(t, engine, vibration(6.0, base.Add(time.Minute), "evt-2"))
	later := apply(t, engine, vibration(6.5, base.Add(3*time.Minute), "evt-3"))[0]
	if !reflect.DeepEqual(published, want) {
		t.Fatalf("version 1 changed after later features:\n got %+v\nwant %+v", published, want)
	}
	if later.Version != 2 || reflect.DeepEqual(later.Facts, published.Facts) || len(later.Evidence) != 3 {
		t.Fatalf("version 2 = %+v, want a distinct version over three evidence events", later)
	}
}

func TestMutatingAPublishedVersionDoesNotReachTheEngine(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	published := apply(t, engine, vibration(5.0, base, "evt-1"))[0]
	published.Facts["facts.vibration_rms"] = 999.0
	published.Evidence[0] = "tampered"
	published.ConditionStart["watch->warning"] = base
	next := apply(t, engine, vibration(6.0, base.Add(time.Minute), "evt-2"))
	state, _, _, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if state.Facts["facts.vibration_rms"] != 6.0 || len(next) != 0 {
		t.Fatalf("state facts = %v, published %+v: the caller's edit leaked into the engine", state.Facts, next)
	}
	if _, tampered := state.Evidence["tampered"]; tampered {
		t.Fatalf("evidence = %v, want the caller's edit ignored", state.Evidence)
	}
	if _, leaked := state.ConditionStart["watch->warning"]; !leaked || !state.ConditionStart["watch->warning"].Equal(base.Add(time.Minute)) {
		t.Fatalf("condition start = %v, want the engine's own instant %s", state.ConditionStart, base.Add(time.Minute))
	}
}

func TestVersionNumbersOnlyGrowAndEachStepNamesItsPredecessor(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	var versions []domain.Version
	for _, f := range []struct {
		at    time.Duration
		value float64
	}{{0, 5.0}, {time.Minute, 6.0}, {3 * time.Minute, 6.0}, {4 * time.Minute, 2.0}} {
		versions = append(versions, apply(t, engine, vibration(f.value, base.Add(f.at)))...)
	}
	for i, v := range versions {
		if v.Version != i+1 || v.PreviousVersion != i {
			t.Fatalf("version %d = %d (previous %d), want %d (previous %d)", i, v.Version, v.PreviousVersion, i+1, i)
		}
	}
	if len(versions) != 3 {
		t.Fatalf("published %d versions, want 3 (watch, warning, resolved)", len(versions))
	}
}
