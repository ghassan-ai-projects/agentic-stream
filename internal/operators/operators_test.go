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
