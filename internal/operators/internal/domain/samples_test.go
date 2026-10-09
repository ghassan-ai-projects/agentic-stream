package domain_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestOnlyValidFiniteNumericObservationsEnterAWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    map[string]any
		quality []contractsv1.QualityFlag
		want    int
	}{
		{"valid payload quality", map[string]any{"celsius": 42.0, "quality": "valid"}, nil, 1},
		{"legacy payload without quality", map[string]any{"celsius": 42.0}, nil, 1},
		{"invalid payload quality", map[string]any{"celsius": 42.0, "quality": "invalid"}, nil, 0},
		{"warming payload quality", map[string]any{"celsius": 42.0, "quality": "warming"}, nil, 0},
		{"disconnected payload quality", map[string]any{"celsius": 42.0, "quality": "disconnected"}, nil, 0},
		{"rail fault payload quality", map[string]any{"celsius": 42.0, "quality": "rail_high"}, nil, 0},
		{"non-string payload quality", map[string]any{"celsius": 42.0, "quality": true}, nil, 0},
		{"envelope quality flag", map[string]any{"celsius": 42.0}, []contractsv1.QualityFlag{{Code: "disconnected"}}, 0},
		{"NaN value", map[string]any{"celsius": math.NaN(), "quality": "valid"}, nil, 0},
		{"infinite value", map[string]any{"celsius": math.Inf(1), "quality": "valid"}, nil, 0},
		{"missing field", map[string]any{"quality": "valid"}, nil, 0},
		{"text value", map[string]any{"celsius": "42", "quality": "valid"}, nil, 0},
		{"float32 value", map[string]any{"celsius": float32(42)}, nil, 1},
		{"int value", map[string]any{"celsius": 42}, nil, 1},
		{"int64 value", map[string]any{"celsius": int64(42)}, nil, 1},
		{"json number value", map[string]any{"celsius": json.Number("42.5")}, nil, 1},
		{"unreadable json number", map[string]any{"celsius": json.Number("forty")}, nil, 0},
		{"infinite json number", map[string]any{"celsius": json.Number("1e999")}, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, meanSpec())
			env := envelope("quality", "sensor.temperature", base, tc.data)
			env.Quality = tc.quality
			if got := len(h.applyOnTime(env)); got != tc.want {
				t.Fatalf("features = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOperatorFieldMustNameOneDataKey(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"", "celsius", "meta.celsius", "data.celsius.deep"} {
		t.Run("field="+field, func(t *testing.T) {
			t.Parallel()
			compiled := meanSpec()
			compiled.Operators[0].Field = field
			h := newHarness(t, compiled)
			if features := h.applyOnTime(temperature("t", base, 1)); len(features) != 0 {
				t.Fatalf("field %q produced %+v, want no feature", field, features)
			}
		})
	}
}

func TestQualityAdmissionIsPerOperator(t *testing.T) {
	t.Parallel()
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs:        []spec.Input{{Name: "zone", EventType: "zone.temp.observed", SchemaVersion: "1.0", SchemaRef: "zone.temp.observed/1.0", PartitionKey: "entity.id", EntityType: "zone"}},
		Windows:       []spec.Window{{Name: "w1", Kind: "tumbling", Size: "5m", Emit: "on_update"}},
		Operators: []spec.Operator{
			{Name: "temperature", Kind: "aggregate", Inputs: []string{"zone"}, Field: "data.celsius", Window: "w1", Aggregate: "mean", Output: "temperature_mean"},
			{Name: "heartbeat", Kind: "missing_heartbeat", Inputs: []string{"zone"}, Duration: "5m", Output: "heartbeat_missing"},
		},
	}
	h := newHarness(t, compiled)
	withoutBoot := envelope("zone-event", "zone.temp.observed", base, map[string]any{"celsius": 25.0, "quality": "valid"})
	features := h.applyOnTime(withoutBoot)
	if len(features) != 1 || features[0].OperatorID != "heartbeat" {
		t.Fatalf("features = %+v, want only the heartbeat feature: the schema requires a boot id for numeric observations", features)
	}
}
