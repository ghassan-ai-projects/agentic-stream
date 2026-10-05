package domain

import (
	"errors"
	"fmt"
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

func TestEpochRefusalRetainsSentinelCause(t *testing.T) {
	t.Parallel()
	unbound, killed, unexpected := errors.New("unbound"), errors.New("killed"), errors.New("store failed")
	for _, test := range []struct {
		err    error
		reason string
	}{{nil, ""}, {fmt.Errorf("wrapped: %w", unbound), "epoch_unbound"}, {fmt.Errorf("wrapped: %w", killed), "epoch_killed"}} {
		reason, err := DecisionEpochRefusal(test.err, unbound, killed)
		if err != nil || reason != test.reason {
			t.Fatalf("refusal=%s error=%v", reason, err)
		}
	}
	if reason, err := DecisionEpochRefusal(unexpected, unbound, killed); reason != "" || !errors.Is(err, unexpected) {
		t.Fatalf("unexpected refusal=%s error=%v", reason, err)
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
