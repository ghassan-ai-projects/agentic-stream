package domain_test

import (
	"slices"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
)

func testSample(id string, at time.Duration) domain.Sample {
	return domain.Sample{EventID: id, EventTime: base.Add(at), Value: 1}
}

func sampleIDs(samples []domain.Sample) []string {
	ids := make([]string, len(samples))
	for i, s := range samples {
		ids[i] = s.EventID
	}
	return ids
}

func TestAWindowReportsTheSamplesAdmittedAndExpiredSinceTheLastSave(t *testing.T) {
	t.Parallel()
	window := &domain.WindowState{}
	window.RestoreSamples([]domain.Sample{testSample("a", 0), testSample("b", time.Minute)})
	window.AddSample(testSample("c", 2*time.Minute), base.Add(30*time.Second))
	admitted, expired := window.UnsavedSamples()
	if !slices.Equal(sampleIDs(admitted), []string{"c"}) || !slices.Equal(sampleIDs(expired), []string{"a"}) {
		t.Fatalf("admitted %v expired %v, want c admitted and a expired", sampleIDs(admitted), sampleIDs(expired))
	}
	if !slices.Equal(sampleIDs(window.Samples), []string{"b", "c"}) {
		t.Fatalf("samples = %v, want b and c", sampleIDs(window.Samples))
	}
	window.SamplesSaved()
	if admitted, expired := window.UnsavedSamples(); len(admitted)+len(expired) != 0 {
		t.Fatalf("admitted %v expired %v after a save, want none", admitted, expired)
	}
}

func TestASampleOlderThanTheWindowIsNeverReportedAsAdmitted(t *testing.T) {
	t.Parallel()
	window := &domain.WindowState{}
	window.AddSample(testSample("late", 0), base.Add(time.Minute))
	admitted, expired := window.UnsavedSamples()
	if len(admitted)+len(expired) != 0 || len(window.Samples) != 0 {
		t.Fatalf("admitted %v expired %v samples %v, want the stale sample to leave no trace", admitted, expired, window.Samples)
	}
}

func TestRestoredSamplesComeBackInEventTimeOrder(t *testing.T) {
	t.Parallel()
	window := &domain.WindowState{}
	window.RestoreSamples([]domain.Sample{testSample("b", time.Minute), testSample("z", 0), testSample("a", 0)})
	if got := sampleIDs(window.Samples); !slices.Equal(got, []string{"a", "z", "b"}) {
		t.Fatalf("restored order = %v, want event time then event id", got)
	}
}
