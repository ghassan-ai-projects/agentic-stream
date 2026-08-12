package situations_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestSituationTransitionsOnFeature(t *testing.T) {
	ctx := context.Background()
	compiled := spec.CompiledSpec{
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
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
				{From: "watch", To: "warning", When: "features.vibration_rms > 5.5", MinDuration: "0s"},
			},
			Reducers: []spec.Reducer{
				{Field: "facts.vibration_rms", Strategy: "latest_event_time", Input: "vibration_rms"},
			},
		},
	}

	eng, err := situations.NewEngine("d1", "default", 0, &compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First feature opens the situation and moves to watch.
	versions, err := eng.ApplyFeature(ctx, operators.Feature{
		OutputName:    "vibration_rms",
		EntityType:    "motor",
		EntityID:      "motor-17",
		Value:         5.0,
		EventTime:     base,
		Watermark:     base,
		InputEventIDs: []string{"evt-1"},
	}, base)
	if err != nil {
		t.Fatalf("ApplyFeature: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected a situation version")
	}
	if versions[0].Phase != "watch" {
		t.Fatalf("expected phase watch, got %s", versions[0].Phase)
	}

	// Second feature moves to warning.
	versions, err = eng.ApplyFeature(ctx, operators.Feature{
		OutputName:    "vibration_rms",
		EntityType:    "motor",
		EntityID:      "motor-17",
		Value:         6.0,
		EventTime:     base.Add(time.Minute),
		Watermark:     base.Add(time.Minute),
		InputEventIDs: []string{"evt-2"},
	}, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("ApplyFeature: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected a situation version")
	}
	if versions[0].Phase != "warning" {
		t.Fatalf("expected phase warning, got %s", versions[0].Phase)
	}
}
