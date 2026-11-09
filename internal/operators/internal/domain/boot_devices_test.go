package domain_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func twoDeviceSpec() *spec.CompiledSpec {
	input := func(name string) spec.Input {
		return spec.Input{Name: name, EventType: "sensor." + name, SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"}
	}
	operator := func(name, aggregate string, inputs ...string) spec.Operator {
		return spec.Operator{Name: name, Kind: "aggregate", Aggregate: aggregate, Inputs: inputs, Field: "data.celsius", Window: "w1", Output: name}
	}
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs:        []spec.Input{input("left"), input("right")},
		Windows:       []spec.Window{{Name: "w1", Kind: "sliding", Size: "5m", Slide: "1m", Emit: "on_update"}},
		Operators:     []spec.Operator{operator("left_max", "max", "left"), operator("right_max", "max", "right"), operator("both_count", "count", "left", "right")},
	}
}

type deviceReading struct {
	side, boot string
	celsius    float64
}

func applyReadings(h *harness, readings []deviceReading) map[string][]domain.Feature {
	emitted := map[string][]domain.Feature{}
	for i, reading := range readings {
		id := fmt.Sprintf("%s-%d", reading.side, i)
		at := base.Add(time.Duration(i) * time.Second)
		emitted[id] = h.applyOnTime(envelope(id, "sensor."+reading.side, at, bootTemperature(reading.celsius, reading.boot, uint64(i))))
	}
	return emitted
}

func featureOf(features []domain.Feature, output string) (domain.Feature, bool) {
	index := slices.IndexFunc(features, func(f domain.Feature) bool { return f.OutputName == output })
	if index < 0 {
		return domain.Feature{}, false
	}
	return features[index], true
}

func TestTwoDevicesOfOneEntityKeepTheirOwnBootsWithoutDiscardingEachOther(t *testing.T) {
	t.Parallel()
	h := newHarness(t, twoDeviceSpec())
	emitted := applyReadings(h, []deviceReading{{"left", "L1", 1}, {"right", "R1", 10}, {"left", "L1", 2}, {"right", "R1", 20}, {"left", "L1", 3}, {"right", "R1", 30}})
	left, emittedLeft := featureOf(emitted["left-4"], "left_max")
	if !emittedLeft || left.Value != 3.0 || !slices.Equal(left.InputEventIDs, []string{"left-0", "left-2", "left-4"}) {
		t.Fatalf("left features = %+v, want max 3 over the three left readings", emitted["left-4"])
	}
	right, emittedRight := featureOf(emitted["right-5"], "right_max")
	if !emittedRight || right.Value != 30.0 || !slices.Equal(right.InputEventIDs, []string{"right-1", "right-3", "right-5"}) {
		t.Fatalf("right features = %+v, want max 30 over the three right readings", emitted["right-5"])
	}
}

func TestARebootOfOneDeviceFencesItsStaleEventsAndKeepsTheOtherDevicesState(t *testing.T) {
	t.Parallel()
	h := newHarness(t, twoDeviceSpec())
	emitted := applyReadings(h, []deviceReading{{"left", "L1", 10}, {"right", "R1", 20}, {"left", "L2", 5}, {"left", "L1", 99}, {"right", "R1", 30}})
	if left, ok := featureOf(emitted["left-2"], "left_max"); !ok || left.Value != 5.0 || !slices.Equal(left.InputEventIDs, []string{"left-2"}) {
		t.Fatalf("features after the left reboot = %+v, want max 5 over the new boot's reading only", emitted["left-2"])
	}
	if stale := emitted["left-3"]; len(stale) != 0 {
		t.Fatalf("the retired left boot emitted %+v, want nothing", stale)
	}
	if right, ok := featureOf(emitted["right-4"], "right_max"); !ok || right.Value != 30.0 || !slices.Equal(right.InputEventIDs, []string{"right-1", "right-4"}) {
		t.Fatalf("right features = %+v, want max 30 over both right readings: the left reboot must not discard them", emitted["right-4"])
	}
}

func TestAnOperatorFedBySeveralEventTypesLosesItsStateWhenAnyOfThemAdmitsANewBoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, twoDeviceSpec())
	emitted := applyReadings(h, []deviceReading{{"left", "L1", 1}, {"left", "L1", 2}, {"right", "R1", 3}, {"left", "L1", 4}})
	if count, ok := featureOf(emitted["left-1"], "both_count"); !ok || count.Value != 2.0 {
		t.Fatalf("count before the right boot = %+v, want 2", emitted["left-1"])
	}
	if count, ok := featureOf(emitted["left-3"], "both_count"); !ok || count.Value != 1.0 {
		t.Fatalf("count after the right boot = %+v, want 1: the right boot discards the state it feeds", emitted["left-3"])
	}
}

func TestTimerStateOfEveryDevicesCurrentBootStaysActive(t *testing.T) {
	t.Parallel()
	h := newHarness(t, twoDeviceSpec())
	applyReadings(h, []deviceReading{{"left", "L1", 1}, {"right", "R1", 2}, {"left", "L2", 3}})
	for stateKey, want := range map[string]bool{"motor-17\x1fL2": true, "motor-17\x1fR1": true, "motor-17\x1fL1": false, "motor-17": false} {
		if got := h.rt.IsTimerStateActive(h.ps, stateKey); got != want {
			t.Errorf("IsTimerStateActive(%q) = %v, want %v", stateKey, got, want)
		}
	}
}
