package domain_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type harness struct {
	t  *testing.T
	rt *domain.OperatorRuntime
	ps *domain.PartitionState
}

func newHarness(t *testing.T, compiled *spec.CompiledSpec) *harness {
	t.Helper()
	rt, err := domain.NewOperatorRuntime("d1", compiled, sources.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	return &harness{t: t, rt: rt, ps: &domain.PartitionState{}}
}

func (h *harness) apply(env contractsv1.Envelope, watermark, processing time.Time) []domain.Feature {
	h.t.Helper()
	features, ps, err := h.rt.ApplyEventAt(h.t.Context(), h.ps, env, watermark, processing)
	if err != nil {
		h.t.Fatalf("ApplyEventAt(%s): %v", env.ID, err)
	}
	h.ps = ps
	return features
}

func (h *harness) applyOnTime(env contractsv1.Envelope) []domain.Feature {
	h.t.Helper()
	return h.apply(env, env.EventTime, env.IngestedAt)
}

func (h *harness) fireTimer(at time.Time) []domain.Feature {
	h.t.Helper()
	features, ps, err := h.rt.ApplyTimer(h.t.Context(), h.ps, at, at)
	if err != nil {
		h.t.Fatalf("ApplyTimer: %v", err)
	}
	h.ps = ps
	return features
}

func envelope(id, eventType string, eventTime time.Time, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID:             id,
		Type:           eventType,
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      eventTime,
		IngestedAt:     eventTime,
		Classification: contractsv1.ClassificationInternal,
		Data:           data,
	}
}

func temperature(id string, at time.Time, celsius any) contractsv1.Envelope {
	return envelope(id, "sensor.temperature", at, map[string]any{"celsius": celsius, "quality": "valid"})
}

func heartbeat(id string, at time.Time, data map[string]any) contractsv1.Envelope {
	return envelope(id, "test.heartbeat", at, data)
}

func temperatureSpec(window spec.Window, operator spec.Operator) *spec.CompiledSpec {
	operator.Name, operator.Inputs, operator.Field, operator.Window = "op1", []string{"temp"}, "data.celsius", "w1"
	window.Name = "w1"
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Windows:   []spec.Window{window},
		Operators: []spec.Operator{operator},
	}
}

func slidingWindow(size, slide, emit string) spec.Window {
	return spec.Window{Kind: "sliding", Size: size, Slide: slide, Emit: emit}
}

func meanSpec() *spec.CompiledSpec {
	return temperatureSpec(slidingWindow("5m", "1m", "on_update"), spec.Operator{Kind: "aggregate", Aggregate: "mean", Output: "mean_value", Unit: "celsius"})
}

func aggregateSpec(aggregate string) *spec.CompiledSpec {
	return temperatureSpec(slidingWindow("5m", "1m", "on_update"), spec.Operator{Kind: "aggregate", Aggregate: aggregate, Output: aggregate + "_value"})
}

func heartbeatSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "heartbeat", EventType: "test.heartbeat", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Operators: []spec.Operator{
			{Name: "heartbeat_missing", Kind: "missing_heartbeat", Inputs: []string{"heartbeat"}, Duration: "5m", Output: "heartbeat_missing_5m"},
		},
	}
}
