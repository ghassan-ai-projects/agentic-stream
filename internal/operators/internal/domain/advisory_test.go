package domain_test

import (
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func heartbeatBesideTemperature(role string) *spec.CompiledSpec {
	compiled := heartbeatSpec()
	compiled.Inputs[0].CompletenessRole = role
	compiled.Inputs = append(compiled.Inputs, spec.Input{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"})
	return compiled
}

func heartbeatFeatures(features []domain.Feature) []domain.Feature {
	var heartbeats []domain.Feature
	for _, feature := range features {
		if feature.OutputName == "heartbeat_missing_5m" {
			heartbeats = append(heartbeats, feature)
		}
	}
	return heartbeats
}

func TestALostAdvisoryInputIsJudgedInEventTimeWhileTheLinesOtherInputsMoveTheWatermark(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatBesideTemperature("advisory"))
	h.apply(heartbeat("hb", base, nil), base, base)
	if got := heartbeatFeatures(h.apply(temperature("t1", base.Add(4*time.Minute), 20.0), base.Add(4*time.Minute), base)); len(got) != 0 {
		t.Fatalf("heartbeat features before its duration = %+v, want none", got)
	}
	stale := heartbeatFeatures(h.apply(temperature("t2", base.Add(5*time.Minute), 20.0), base.Add(5*time.Minute), base))
	if len(stale) != 1 || stale[0].Value != true || stale[0].Completeness != string(domain.CompletenessUncertain) || !stale[0].EventTime.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("heartbeat features at its duration = %+v, want one missing at the last event plus 5m, though the processing clock never moved", stale)
	}
	if again := heartbeatFeatures(h.apply(temperature("t3", base.Add(9*time.Minute), 20.0), base.Add(9*time.Minute), base)); len(again) != 0 {
		t.Fatalf("heartbeat features once stale = %+v, want none: the loss is reported once", again)
	}
	back := heartbeatFeatures(h.apply(heartbeat("hb2", base.Add(10*time.Minute), nil), base.Add(10*time.Minute), base))
	if len(back) != 1 || back[0].Value != false {
		t.Fatalf("heartbeat features when the input returns = %+v, want one present", back)
	}
	if relost := heartbeatFeatures(h.apply(temperature("t4", base.Add(15*time.Minute), 20.0), base.Add(15*time.Minute), base)); len(relost) != 1 {
		t.Fatalf("heartbeat features after a second loss = %+v, want it reported again", relost)
	}
}

func TestALostRequiredInputWaitsForItsProcessingTimeTimer(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"", "required"} {
		h := newHarness(t, heartbeatBesideTemperature(role))
		h.apply(heartbeat("hb", base, nil), base, base)
		if got := heartbeatFeatures(h.apply(temperature("t1", base.Add(9*time.Minute), 20.0), base.Add(9*time.Minute), base)); len(got) != 0 {
			t.Fatalf("role %q: heartbeat features from another input's event = %+v, want none: only an advisory input is judged in event time", role, got)
		}
	}
}
