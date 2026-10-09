package domain

import (
	"testing"
	"time"
)

func TestPercentileOrdersRecordedDurations(t *testing.T) {
	t.Parallel()
	hundred := make([]time.Duration, 0, 100)
	for i := 1; i <= 100; i++ {
		hundred = append(hundred, time.Duration(i)*time.Millisecond)
	}
	tests := []struct {
		name      string
		durations []time.Duration
		percent   float64
		want      time.Duration
	}{
		{"p50 of 1..100ms", hundred, 50, 50 * time.Millisecond},
		{"p95 of 1..100ms", hundred, 95, 95 * time.Millisecond},
		{"p99 of 1..100ms", hundred, 99, 99 * time.Millisecond},
		{"p0 is the smallest of unordered input", []time.Duration{30, 10, 20}, 0, 10},
		{"p100 is the largest of unordered input", []time.Duration{30, 10, 20}, 100, 30},
		{"no durations", nil, 95, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runtime := NewRuntime(time.Unix(1, 0), nil)
			for _, duration := range tc.durations {
				runtime.ObserveDuration(duration)
			}
			if got := runtime.Percentile(tc.percent); got != tc.want {
				t.Fatalf("Percentile(%v) = %v, want %v", tc.percent, got, tc.want)
			}
		})
	}
}

func TestLatencySnapshotReportsNanosecondsAndClampsNegativeDurations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		durations []time.Duration
		want      uint64
	}{
		{"nothing recorded yet", nil, 0},
		{"one duration", []time.Duration{1500 * time.Microsecond}, 1_500_000},
		{"an anomalous clock cannot wrap the metric", []time.Duration{-time.Second}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runtime := NewRuntime(time.Unix(1, 0), nil)
			for _, duration := range tc.durations {
				runtime.ObserveDuration(duration)
			}
			snapshot := runtime.LatencySnapshot()
			for _, name := range []string{"p50", "p95", "p99"} {
				if got := snapshot["agentic_stream_dispatch_decision_"+name+"_ns"]; got != tc.want {
					t.Errorf("%s = %d, want %d", name, got, tc.want)
				}
			}
			if len(snapshot) != 3 {
				t.Errorf("latency snapshot has %d entries, want 3", len(snapshot))
			}
		})
	}
}

func TestLatencyPercentilesCoverOnlyTheMostRecentDurations(t *testing.T) {
	t.Parallel()
	runtime := NewRuntime(time.Unix(1, 0), nil)
	for range MaxRecentDurations {
		runtime.ObserveDuration(time.Hour)
	}
	for range MaxRecentDurations {
		runtime.ObserveDuration(time.Millisecond)
	}
	if got := runtime.Percentile(99); got != time.Millisecond {
		t.Fatalf("p99 = %s, want the recent millisecond durations only", got)
	}
}
