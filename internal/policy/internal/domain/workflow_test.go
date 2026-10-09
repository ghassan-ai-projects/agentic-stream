package domain

import "testing"

func TestAnOutcomeAuditsItsDetailAndOtherwiseItsStableReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		outcome Outcome
		want    string
	}{
		{Outcome{Reason: "interlock_not_ready"}, "interlock_not_ready"},
		{Outcome{Reason: "interlock_not_ready", AuditDetail: "interlock_not_ready: operator stop"}, "interlock_not_ready: operator stop"},
	}
	for _, test := range tests {
		if got := test.outcome.AuditReason(); got != test.want {
			t.Errorf("AuditReason(%+v) = %q, want %q", test.outcome, got, test.want)
		}
	}
}
