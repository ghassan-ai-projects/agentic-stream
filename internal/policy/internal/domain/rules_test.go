package domain

import "testing"

func TestSourceHealthBlocksOnlyConsequentialIntents(t *testing.T) {
	t.Parallel()
	for _, risk := range []string{"R0", "R1", "R2", "R3", "R4"} {
		for _, health := range []string{"on_time", "provisional", "uncertain"} {
			want := (risk == "R2" || risk == "R3" || risk == "R4") && health != "on_time"
			if got := SourceHealthIncomplete(IntentRecord{RiskClass: risk, CurrentCompleteness: health}); got != want {
				t.Errorf("SourceHealthIncomplete(%s, %s) = %t, want %t", risk, health, got, want)
			}
		}
	}
}

func TestAHumanDecisionMapsToApprovalAndIntentStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		approved               bool
		wantStatus, wantPolicy string
	}{
		{true, "approved", "pending"},
		{false, "denied", "denied"},
	}
	for _, test := range tests {
		if status, policy := ApprovalDecision(test.approved); status != test.wantStatus || policy != test.wantPolicy {
			t.Errorf("ApprovalDecision(%t) = %s/%s, want %s/%s", test.approved, status, policy, test.wantStatus, test.wantPolicy)
		}
	}
}
