package domain

import (
	"math"
	"testing"
	"time"
)

func TestWindowAggregatesKeepNumericAndEmptySemantics(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []Sample{{EventTime: start, Value: -3}, {EventTime: start.Add(time.Hour), Value: 4}}
	for _, tc := range []struct {
		name string
		want float64
	}{
		{"mean", 0.5}, {"rms", math.Sqrt(12.5)}, {"slope", 7}, {"count", 2},
		{"sum", 1}, {"min", -3}, {"max", 4}, {"latest", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value, err := computeAggregate(tc.name, samples)
			if err != nil || value != tc.want {
				t.Fatalf("aggregate=%g want=%g err=%v", value, tc.want, err)
			}
			value, err = computeAggregate(tc.name, nil)
			if err != nil || value != 0 {
				t.Fatalf("empty aggregate=%g err=%v", value, err)
			}
		})
	}
	if _, err := computeAggregate("unknown", samples); err == nil {
		t.Fatal("unknown nonempty aggregate accepted")
	}
	if value, err := computeAggregate("unknown", nil); err != nil || value != 0 {
		t.Fatalf("empty input must precede aggregate validation: value=%g err=%v", value, err)
	}
}

func TestSlopeIsZeroWithoutSpreadInTime(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, samples := range map[string][]Sample{
		"one sample":               {{EventTime: at, Value: 5}},
		"samples at the same time": {{EventTime: at, Value: 5}, {EventTime: at, Value: 9}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if value, err := computeAggregate("slope", samples); err != nil || value != 0 {
				t.Fatalf("slope = %g err %v, want 0", value, err)
			}
		})
	}
}
