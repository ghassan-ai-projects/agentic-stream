package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestEventOfAnUndeclaredTypeProducesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	if features := h.applyOnTime(envelope("x", "sensor.pressure", base, map[string]any{"celsius": 1.0})); len(features) != 0 {
		t.Fatalf("features = %+v, want none", features)
	}
}

func TestOperatorOnlyConsumesItsFirstDeclaredInput(t *testing.T) {
	t.Parallel()
	compiled := meanSpec()
	compiled.Inputs = append(compiled.Inputs, spec.Input{Name: "other", EventType: "sensor.other", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"})
	compiled.Operators[0].Inputs = []string{"temp", "other"}
	h := newHarness(t, compiled)
	if features := h.applyOnTime(envelope("o", "sensor.other", base, map[string]any{"celsius": 1.0})); len(features) != 0 {
		t.Fatalf("features from a secondary input = %+v, want none", features)
	}
	if features := h.applyOnTime(temperature("t", base, 1)); len(features) != 1 {
		t.Fatalf("features from the first input = %d, want 1", len(features))
	}
}

func TestApplyEventCreatesPartitionStateWhenNoneIsGiven(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	features, ps, err := h.rt.ApplyEventAt(t.Context(), nil, temperature("t", base, 1), base, base)
	if err != nil || len(features) != 1 || ps == nil || len(ps.OperatorStates["op1"]) != 1 {
		t.Fatalf("features %d state %+v err %v, want one feature and a populated state", len(features), ps, err)
	}
}

func TestApplyEventHonorsCancellation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := h.rt.ApplyEventAt(ctx, h.ps, temperature("t", base, 1), base, base)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyEventAt error = %v, want %v", err, context.Canceled)
	}
	if len(h.ps.OperatorStates) != 0 {
		t.Fatalf("a canceled event changed state: %+v", h.ps.OperatorStates)
	}
}
