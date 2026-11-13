package domain

import (
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestAFailedReasoningIsRetriedOnceAfterItsEventTimeBackoffWhileItsPhaseHoldsAndUnderItsCap(t *testing.T) {
	t.Parallel()
	policy := spec.RetryPolicy{After: "6m", Max: 2}
	failedAt := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	once := []ReasonedVersion{{Phase: "degrading", Version: 3, Horizon: failedAt}}
	twice := append(slices.Clone(once), ReasonedVersion{Phase: "degrading", Version: 4, Horizon: failedAt.Add(10 * time.Minute)})
	thrice := append(slices.Clone(twice), ReasonedVersion{Phase: "degrading", Version: 5, Horizon: failedAt.Add(20 * time.Minute)})
	at := func(phase string, version int, after time.Duration) situations.Position {
		return situations.Position{Phase: phase, Version: version, EventTime: failedAt.Add(after)}
	}
	for _, tc := range []struct {
		name     string
		at       situations.Position
		reasoned []ReasonedVersion
		want     bool
	}{
		{"before the backoff", at("degrading", 3, 5*time.Minute), once, false},
		{"at the backoff", at("degrading", 3, 6*time.Minute), once, true},
		{"a later version of another cause does not stop the retry", at("degrading", 4, time.Hour), once, true},
		{"the second failure counts from its own version", at("degrading", 4, 15*time.Minute), twice, false},
		{"the second retry", at("degrading", 4, 16*time.Minute), twice, true},
		{"past the cap", at("degrading", 5, time.Hour), thrice, false},
		{"the phase moved on", at("shedding", 3, time.Hour), once, false},
		{"nothing failed", at("degrading", 3, time.Hour), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, due, err := RetryDue(policy, tc.at, tc.reasoned, 0); err != nil || due != tc.want {
				t.Fatalf("RetryDue = %v, %v; want %v", due, err, tc.want)
			}
		})
	}
}

func TestAFailedReasoningIsRetriedOnlyOnce(t *testing.T) {
	t.Parallel()
	policy := spec.RetryPolicy{After: "6m", Max: 2}
	failedAt := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	reasoned := []ReasonedVersion{{Phase: "degrading", Version: 3, Horizon: failedAt}}
	at := situations.Position{Phase: "degrading", Version: 4, EventTime: failedAt.Add(time.Hour)}
	if failed, due, err := RetryDue(policy, at, reasoned, 0); err != nil || !due || failed != 3 {
		t.Fatalf("first judgement = %d, %v, %v; want version 3 due", failed, due, err)
	}
	if _, due, err := RetryDue(policy, at, reasoned, 3); err != nil || due {
		t.Fatalf("a failure already retried is due again (%v)", err)
	}
}

func TestFailedSituationsListsEachSituationOnce(t *testing.T) {
	t.Parallel()
	failed := []FailedReasoning{{"sit-1", 3}, {"sit-1", 4}, {"sit-2", 2}}
	if got := FailedSituations(failed); !slices.Equal(got, []string{"sit-1", "sit-2"}) {
		t.Fatalf("FailedSituations = %v", got)
	}
}
