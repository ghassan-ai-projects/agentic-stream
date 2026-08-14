package operators_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestAggregateMean(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", meanSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		env := contractsv1.Envelope{
			ID:             ids.NewSequence("evt_").New(),
			Type:           "sensor.temperature",
			SchemaVersion:  "1.0",
			TenantID:       "default",
			Source:         "test",
			PartitionKey:   "motor-17",
			Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
			EventTime:      base.Add(time.Duration(i) * time.Minute),
			IngestedAt:     base.Add(time.Duration(i) * time.Minute),
			Classification: contractsv1.ClassificationInternal,
			Data:           map[string]any{"celsius": float64(i + 1)},
		}
		fs, nps, err := rt.ApplyEvent(ctx, ps, env, env.EventTime)
		if err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
		ps = nps
		if i == 2 && len(fs) == 0 {
			t.Fatal("expected feature on third event")
		}
		if i == 2 && len(fs) > 0 {
			if fs[0].OutputName != "mean_value" {
				t.Fatalf("unexpected output %q", fs[0].OutputName)
			}
			if got, want := fs[0].Value, 2.0; got != want {
				t.Fatalf("mean = %v, want %v", got, want)
			}
		}
	}
}

func TestSlope(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", slopeSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		env := contractsv1.Envelope{
			ID:             ids.NewSequence("evt_").New(),
			Type:           "sensor.temperature",
			SchemaVersion:  "1.0",
			TenantID:       "default",
			Source:         "test",
			PartitionKey:   "motor-17",
			Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
			EventTime:      base.Add(time.Duration(i) * time.Hour),
			IngestedAt:     base.Add(time.Duration(i) * time.Hour),
			Classification: contractsv1.ClassificationInternal,
			Data:           map[string]any{"celsius": float64(i)},
		}
		fs, nps, err := rt.ApplyEvent(ctx, ps, env, env.EventTime)
		if err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
		ps = nps
		if i == 2 && len(fs) == 0 {
			t.Fatal("expected slope feature")
		}
	}
}

func TestLateEventCorrectsPreviouslyEmittedWindow(t *testing.T) {
	compiled := meanSpec()
	compiled.Time = spec.TimePolicy{
		MaxOutOfOrderness: "0s",
		AllowedLateness:   "5m",
		LatePolicy:        "correct_and_reconsider",
	}
	rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := contractsv1.Envelope{
		ID:             "evt-first",
		Type:           "sensor.temperature",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      base,
		IngestedAt:     base,
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"celsius": 10.0},
	}
	if _, ps, err = rt.ApplyEvent(ctx, ps, first, base); err != nil {
		t.Fatalf("apply first event: %v", err)
	}

	late := first
	late.ID = "evt-late"
	late.EventTime = base.Add(-time.Minute)
	features, _, err := rt.ApplyEvent(ctx, ps, late, base)
	if err != nil {
		t.Fatalf("apply late event: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("late event emitted %d features, want 1", len(features))
	}
	if got := features[0].Completeness; got != string(operators.CompletenessCorrected) {
		t.Fatalf("late feature completeness = %q, want corrected", got)
	}
	if got, want := features[0].Value, 10.0; got != want {
		t.Fatalf("late feature value = %v, want %v", got, want)
	}
	if got, want := len(features[0].InputEventIDs), 2; got != want {
		t.Fatalf("late feature input event count = %d, want %d", got, want)
	}

	inOrder := first
	inOrder.ID = "evt-in-order"
	inOrder.EventTime = base.Add(time.Minute)
	features, _, err = rt.ApplyEvent(ctx, ps, inOrder, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("apply in-order event: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("in-order event emitted %d features, want 1", len(features))
	}
	if got := features[0].Completeness; got == string(operators.CompletenessCorrected) {
		t.Fatal("in-order feature was incorrectly marked corrected")
	}
}

func meanSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Windows: []spec.Window{
			{Name: "w1", Kind: "sliding", Size: "5m", Slide: "1m", Emit: "on_update"},
		},
		Operators: []spec.Operator{
			{Name: "op1", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Window: "w1", Aggregate: "mean", Output: "mean_value", Unit: "celsius"},
		},
	}
}

func slopeSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Windows: []spec.Window{
			{Name: "w1", Kind: "sliding", Size: "6h", Slide: "15m", Emit: "on_update"},
		},
		Operators: []spec.Operator{
			{Name: "op1", Kind: "slope", Inputs: []string{"temp"}, Field: "data.celsius", Window: "w1", Aggregate: "slope", Output: "slope_value", Unit: "celsius_per_hour"},
		},
	}
}
