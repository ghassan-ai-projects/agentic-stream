package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestDispatchBindingBoundsStaleRebinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		bound     int
		live      int64
		available bool
		count     int
		want      SnapshotBinding
	}{
		{"current", 2, 2, false, 3, SnapshotCurrent},
		{"refresh", 2, 3, true, 2, SnapshotRebind},
		{"exhausted", 2, 3, true, 3, SnapshotQuarantine},
		{"missing assembler", 2, 3, false, 0, SnapshotQuarantine},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := DispatchBinding(test.bound, test.live, test.available, test.count); got != test.want {
				t.Fatalf("binding=%s want=%s", got, test.want)
			}
		})
	}
}

func TestRetryBudgetNeverRevivesSupersededEpisode(t *testing.T) {
	t.Parallel()
	for _, superseded := range []bool{false, true} {
		for failures := 0; failures <= 3; failures++ {
			if got := ShouldRetry(superseded, failures); got != (!superseded && failures < 3) {
				t.Fatalf("superseded=%v failures=%d retry=%v", superseded, failures, got)
			}
		}
	}
}

func TestOutcomeIdentityRejectsStaleFenceBeforeWrongAttempt(t *testing.T) {
	t.Parallel()
	current := episodeledger.Identity{AttemptID: "current", Fence: 3}
	if got := OutcomeIdentityRejection(current, episodeledger.Identity{AttemptID: "wrong", Fence: 2}); got != episodeledger.RejectStaleAttempt {
		t.Fatalf("stale fence=%s", got)
	}
	if got := OutcomeIdentityRejection(current, episodeledger.Identity{AttemptID: "wrong", Fence: 3}); got != episodeledger.RejectWrongAttempt {
		t.Fatalf("wrong attempt=%s", got)
	}
}

func TestTerminalAttemptStatusGivesDecisionValidationPrecedenceOverExecutorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		outcome       *Outcome
		hasDecision   bool
		validDecision bool
		want          episodeledger.AttemptStatus
	}{
		{name: "valid decision", outcome: &Outcome{Status: "declined"}, hasDecision: true, validDecision: true, want: episodeledger.AttemptProduced},
		{name: "rejected decision", outcome: &Outcome{}, hasDecision: true, want: episodeledger.AttemptFailed},
		{name: "no decision, no status", outcome: &Outcome{}, want: episodeledger.AttemptDeclined},
		{name: "no decision, executor status", outcome: &Outcome{Status: string(episodeledger.AttemptTimedOut)}, want: episodeledger.AttemptTimedOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := TerminalAttemptStatus(tt.outcome, tt.hasDecision, tt.validDecision); got != tt.want {
				t.Fatalf("terminalAttemptStatus = %q, want %q", got, tt.want)
			}
		})
	}
}
