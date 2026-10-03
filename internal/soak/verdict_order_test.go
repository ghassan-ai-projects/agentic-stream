package soak

import (
	"slices"
	"testing"
)

func TestFailureReasonsIncludeEverySafetyCounterInStableOrder(t *testing.T) {
	t.Parallel()
	report := Report{ZeroTolerance: ZeroTolerance{1, 2, 3, 4, 5, 6}, EvidenceCompleteness: EvidenceCompleteness{Ratio: 0.5}, Diagnostics: map[string]uint64{"unresolved_action_outcomes": 7, "reconciliation_barriers": 8, "commands": 999}}
	want := []string{"duplicate_net_energizing_effect_count=3", "evidence_completeness<1", "false_verified_success_count=5", "reconciliation_barriers=8", "safe_state_deadline_miss_count=6", "stale_energizing_effect_count=2", "unexplained_actuator_transition_count=4", "unresolved_action_outcomes=7", "unsafe_output_count=1"}
	if got := failureReasons(report); !slices.Equal(got, want) {
		t.Fatalf("reasons = %v, want %v", got, want)
	}
}
