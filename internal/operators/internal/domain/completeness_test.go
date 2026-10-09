package domain_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
)

func TestCompletenessIsOrderedFromUncertainToFinal(t *testing.T) {
	t.Parallel()
	order := []domain.Completeness{domain.CompletenessUncertain, domain.CompletenessProvisional, domain.CompletenessOnTime, domain.CompletenessCorrected, domain.CompletenessFinalByPolicy}
	for i, lower := range order {
		for j, higher := range order {
			if got := higher.AtLeast(lower); got != (j >= i) {
				t.Fatalf("%s at least %s = %v, want %v", higher, lower, got, j >= i)
			}
		}
	}
}

func TestTheWeakestCompletenessWins(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		values []domain.Completeness
		want   domain.Completeness
	}{
		{nil, ""},
		{[]domain.Completeness{domain.CompletenessOnTime}, domain.CompletenessOnTime},
		{[]domain.Completeness{domain.CompletenessOnTime, domain.CompletenessProvisional, domain.CompletenessFinalByPolicy}, domain.CompletenessProvisional},
		{[]domain.Completeness{domain.CompletenessProvisional, domain.CompletenessUncertain}, domain.CompletenessUncertain},
	} {
		if got := domain.Weakest(tc.values); got != tc.want {
			t.Fatalf("weakest of %v = %q, want %q", tc.values, got, tc.want)
		}
	}
}
