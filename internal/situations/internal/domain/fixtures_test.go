package domain_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const zeroDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func vibrationSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		Digest:        zeroDigest,
		SchemaVersion: "agentic-stream/v1",
		Situation: spec.Situation{
			Type:         "bearing_degradation",
			EntityKey:    "entity.id",
			InitialPhase: "candidate",
			Occurrence: spec.Occurrence{
				OpenWhen:  "features.vibration_rms > 4.5",
				CloseWhen: "features.vibration_rms < 3.0",
			},
			Phases: []spec.Phase{
				{Name: "candidate", Severity: 10},
				{Name: "watch", Severity: 30},
				{Name: "warning", Severity: 60},
			},
			Transitions: []spec.Transition{
				{From: "candidate", To: "watch", When: "features.vibration_rms > 4.5", MinDuration: "0s"},
				{From: "watch", To: "warning", When: "features.vibration_rms > 5.5", MinDuration: "2m"},
			},
			Reducers: []spec.Reducer{
				{Field: "facts.vibration_rms", Strategy: "latest_event_time", Input: "vibration_rms"},
				{Field: "evidence", Strategy: "set_union", Input: "vibration_rms"},
			},
		},
	}
}

func newEngine(t *testing.T, compiled *spec.CompiledSpec) *domain.Engine {
	t.Helper()
	engine, err := domain.NewEngine("d1", "default", 0, compiled, sources.Deterministic())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

func vibration(value float64, at time.Time, eventIDs ...string) operators.Feature {
	return operators.Feature{
		OutputName: "vibration_rms", EntityType: "motor", EntityID: "motor-17",
		Value: value, EventTime: at, Watermark: at, InputEventIDs: eventIDs,
	}
}

func apply(t *testing.T, engine *domain.Engine, feature operators.Feature) []domain.Version {
	t.Helper()
	versions, err := engine.ApplyFeature(t.Context(), feature, feature.Watermark)
	if err != nil {
		t.Fatalf("ApplyFeature(%v at %s): %v", feature.Value, feature.EventTime, err)
	}
	return versions
}
