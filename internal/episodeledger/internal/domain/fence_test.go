package domain

import (
	"errors"
	"strings"
	"testing"
)

func reasonOf(err error) RejectionReason {
	var identityErr *IdentityError
	if errors.As(err, &identityErr) {
		return identityErr.Reason
	}
	return ""
}

func TestAWorkerIdentityIsJudgedAgainstTheRecordedFence(t *testing.T) {
	t.Parallel()
	running := EpisodeFence{Lifecycle: LifecycleRunning, Attempt: "a2", HasAttempt: true, Fence: 2}
	concluded := EpisodeFence{Lifecycle: LifecycleConcluded, Attempt: "a2", HasAttempt: true, Fence: 2}
	for name, test := range map[string]struct {
		fence    EpisodeFence
		identity Identity
		want     RejectionReason
	}{
		"current attempt":           {running, Identity{AttemptID: "a2", Fence: 2}, ""},
		"closed episode":            {concluded, Identity{AttemptID: "a2", Fence: 2}, RejectEpisodeClosed},
		"closed beats stale":        {concluded, Identity{AttemptID: "a1", Fence: 1}, RejectEpisodeClosed},
		"older fence":               {running, Identity{AttemptID: "a1", Fence: 1}, RejectStaleAttempt},
		"other attempt, same fence": {running, Identity{AttemptID: "x", Fence: 2}, RejectWrongAttempt},
		"newer fence":               {running, Identity{AttemptID: "a2", Fence: 3}, RejectWrongAttempt},
		"no attempt started yet":    {EpisodeFence{Lifecycle: LifecycleAdmitted}, Identity{AttemptID: "a", Fence: 0}, RejectWrongAttempt},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := reasonOf(test.fence.CheckOpenIdentity(test.identity)); got != test.want {
				t.Errorf("reason = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCheckIdentityIgnoresTheLifecycle(t *testing.T) {
	t.Parallel()
	closed := EpisodeFence{Lifecycle: LifecycleSuperseded, Attempt: "a", HasAttempt: true, Fence: 1}
	if err := closed.CheckIdentity(Identity{AttemptID: "a", Fence: 1}); err != nil {
		t.Fatalf("a closed episode must still recognize its current attempt: %v", err)
	}
	if got := reasonOf(closed.CheckIdentity(Identity{AttemptID: "a", Fence: 0})); got != RejectStaleAttempt {
		t.Fatalf("an older fence on a closed episode = %q, want %q", got, RejectStaleAttempt)
	}
}

func TestAnAttemptIsOpenOnlyUnderItsOwnerEpochAndWhileNotTerminal(t *testing.T) {
	t.Parallel()
	owned := Identity{OwnerEpoch: "e1"}
	for name, test := range map[string]struct {
		identity Identity
		record   AttemptRecord
		want     RejectionReason
	}{
		"owned and running":              {owned, AttemptRecord{Status: AttemptRunning, OwnerEpoch: "e1", HasOwnerEpoch: true}, ""},
		"owned by another epoch":         {owned, AttemptRecord{Status: AttemptRunning, OwnerEpoch: "e0", HasOwnerEpoch: true}, RejectStaleAttempt},
		"row has no owner epoch":         {owned, AttemptRecord{Status: AttemptRunning}, RejectStaleAttempt},
		"owner mismatch beats terminal":  {owned, AttemptRecord{Status: AttemptProduced, OwnerEpoch: "e0", HasOwnerEpoch: true}, RejectStaleAttempt},
		"owned but terminal":             {owned, AttemptRecord{Status: AttemptProduced, OwnerEpoch: "e1", HasOwnerEpoch: true}, RejectTerminalAttempt},
		"unowned identity, open attempt": {Identity{}, AttemptRecord{Status: AttemptDispatched}, ""},
		"unowned identity, terminal":     {Identity{}, AttemptRecord{Status: AttemptFailed}, RejectTerminalAttempt},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := reasonOf(CheckAttemptOpen(test.identity, test.record)); got != test.want {
				t.Errorf("reason = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEveryTerminalAttemptStatusIsRefusedAsTerminal(t *testing.T) {
	t.Parallel()
	for _, status := range attemptStatuses {
		want := RejectionReason("")
		if IsTerminalAttempt(status) {
			want = RejectTerminalAttempt
		}
		if got := reasonOf(CheckAttemptNotTerminal(status)); got != want {
			t.Errorf("CheckAttemptNotTerminal(%s) reason = %q, want %q", status, got, want)
		}
	}
}

func TestOnlyALiveEpisodeIsStartable(t *testing.T) {
	t.Parallel()
	for _, status := range lifecycleStatuses {
		fence, err := EpisodeFence{Lifecycle: status, Fence: 4}.CheckStartable()
		if status.Live() {
			if err != nil || fence != 4 {
				t.Errorf("%s: fence=%d err=%v, want fence 4 and no error", status, fence, err)
			}
			continue
		}
		if got := reasonOf(err); got != RejectEpisodeClosed {
			t.Errorf("%s: reason = %q, want %q", status, got, RejectEpisodeClosed)
		}
	}
}

func TestANewAttemptWaitsForTheActivePriorAttempt(t *testing.T) {
	t.Parallel()
	for _, status := range attemptStatuses {
		err := CheckPriorAttemptTerminal("e", "a", status)
		if IsTerminalAttempt(status) {
			if err != nil {
				t.Errorf("%s: terminal prior attempt refused: %v", status, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "episode e already has active attempt a") {
			t.Errorf("%s: err = %v, want a refusal naming the episode and attempt", status, err)
		}
	}
}
