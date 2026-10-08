package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAttemptTransitionTable(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		from, to AttemptStatus
		ok       bool
	}{
		{AttemptDispatched, AttemptRunning, true},
		{AttemptDispatched, AttemptProduced, false},
		{AttemptRunning, AttemptProduced, true},
		{AttemptRunning, AttemptDispatched, false},
		{AttemptCancelling, AttemptCancelled, true},
		{AttemptCancelling, AttemptProduced, false},
		{AttemptProduced, AttemptRunning, false},
		{AttemptAbandoned, AttemptFailed, false},
	} {
		if CanTransitionAttempt(test.from, test.to) != test.ok {
			t.Errorf("%s -> %s allowed=%v, want %v", test.from, test.to, !test.ok, test.ok)
		}
		if (CheckTransition(test.from, test.to) == nil) != test.ok {
			t.Errorf("CheckTransition(%s, %s) disagrees with the table", test.from, test.to)
		}
	}
}

func TestTerminalAndClosedStates(t *testing.T) {
	t.Parallel()
	for status, terminal := range map[AttemptStatus]bool{AttemptDispatched: false, AttemptRunning: false, AttemptCancelling: false, AttemptProduced: true, AttemptDeclined: true, AttemptCancelled: true, AttemptFailed: true, AttemptTimedOut: true, AttemptAbandoned: true} {
		if IsTerminalAttempt(status) != terminal {
			t.Errorf("IsTerminalAttempt(%s) = %v", status, !terminal)
		}
		if (KindOf(status) == TransitionTerminal) != terminal {
			t.Errorf("KindOf(%s) disagrees with IsTerminalAttempt", status)
		}
	}
	if KindOf(AttemptRunning) != TransitionRunning || KindOf(AttemptCancelling) != TransitionStatusOnly {
		t.Fatal("transition kinds changed")
	}
	for status, closed := range map[LifecycleStatus]bool{LifecycleAdmitted: false, LifecycleRunning: false, LifecycleConcluded: true, LifecycleClosed: true, LifecycleSuperseded: true, LifecycleExpired: true, LifecycleAbandoned: true} {
		if status.Closed() != closed {
			t.Errorf("%s.Closed() = %v", status, !closed)
		}
	}
	if !MayAcknowledgeCancellation(AttemptCancelled) || !MayAcknowledgeCancellation(AttemptAbandoned) || MayAcknowledgeCancellation(AttemptProduced) {
		t.Fatal("cancellation acknowledgement rule changed")
	}
}

func reason(err error) RejectionReason {
	var identityErr *IdentityError
	if errors.As(err, &identityErr) {
		return identityErr.Reason
	}
	return ""
}

func TestFenceChecksIdentityBeforeAnything(t *testing.T) {
	t.Parallel()
	open := EpisodeFence{Lifecycle: LifecycleRunning, Attempt: "a2", HasAttempt: true, Fence: 2}
	for name, test := range map[string]struct {
		fence    EpisodeFence
		identity Identity
		want     RejectionReason
	}{
		"current":        {open, Identity{AttemptID: "a2", Fence: 2}, ""},
		"closed":         {EpisodeFence{Lifecycle: LifecycleConcluded, Attempt: "a2", HasAttempt: true, Fence: 2}, Identity{AttemptID: "a2", Fence: 2}, RejectEpisodeClosed},
		"older fence":    {open, Identity{AttemptID: "a1", Fence: 1}, RejectStaleAttempt},
		"other attempt":  {open, Identity{AttemptID: "x", Fence: 2}, RejectWrongAttempt},
		"newer fence":    {open, Identity{AttemptID: "a2", Fence: 3}, RejectWrongAttempt},
		"no attempt yet": {EpisodeFence{Lifecycle: LifecycleAdmitted}, Identity{AttemptID: "a", Fence: 0}, RejectWrongAttempt},
	} {
		err := test.fence.CheckOpenIdentity(test.identity)
		if reason(err) != test.want {
			t.Errorf("%s: reason = %q, want %q", name, reason(err), test.want)
		}
	}
	if err := (EpisodeFence{Lifecycle: LifecycleConcluded, Attempt: "a", HasAttempt: true}).CheckIdentity(Identity{AttemptID: "a"}); err != nil {
		t.Fatalf("CheckIdentity must ignore the lifecycle: %v", err)
	}
}

func TestAttemptOpenChecksOwnerEpochThenTerminal(t *testing.T) {
	t.Parallel()
	owned := Identity{OwnerEpoch: "e1"}
	for name, test := range map[string]struct {
		identity Identity
		record   AttemptRecord
		want     RejectionReason
	}{
		"owned and running":    {owned, AttemptRecord{Status: AttemptRunning, OwnerEpoch: "e1", HasOwnerEpoch: true}, ""},
		"other owner":          {owned, AttemptRecord{Status: AttemptRunning, OwnerEpoch: "e0", HasOwnerEpoch: true}, RejectStaleAttempt},
		"unowned row":          {owned, AttemptRecord{Status: AttemptRunning}, RejectStaleAttempt},
		"terminal":             {owned, AttemptRecord{Status: AttemptProduced, OwnerEpoch: "e1", HasOwnerEpoch: true}, RejectTerminalAttempt},
		"identity without own": {Identity{}, AttemptRecord{Status: AttemptDispatched}, ""},
	} {
		if got := reason(CheckAttemptOpen(test.identity, test.record)); got != test.want {
			t.Errorf("%s: reason = %q, want %q", name, got, test.want)
		}
	}
}

func TestStartableEpisodeAndPriorAttempt(t *testing.T) {
	t.Parallel()
	if fence, err := (EpisodeFence{Lifecycle: LifecycleAdmitted, Fence: 4}).CheckStartable(); err != nil || fence != 4 {
		t.Fatalf("fence=%d err=%v", fence, err)
	}
	if _, err := (EpisodeFence{Lifecycle: LifecycleSuperseded}).CheckStartable(); reason(err) != RejectEpisodeClosed {
		t.Fatalf("closed episode: %v", err)
	}
	if err := CheckPriorAttemptTerminal("e", "a", AttemptRunning); err == nil || !strings.Contains(err.Error(), "already has active attempt") {
		t.Fatalf("active prior attempt: %v", err)
	}
	if err := CheckPriorAttemptTerminal("e", "a", AttemptFailed); err != nil {
		t.Fatalf("terminal prior attempt refused: %v", err)
	}
}

func TestRejectionRules(t *testing.T) {
	t.Parallel()
	if CheckRejectionReason(RejectStaleAttempt) != nil || CheckRejectionReason("made_up") == nil {
		t.Fatal("rejection reason registry changed")
	}
	if string(RejectionDetails(nil)) != "{}" || string(RejectionDetails([]byte(`{"a":1}`))) != `{"a":1}` {
		t.Fatal("details default changed")
	}
	identity := Identity{EpisodeID: "e", AttemptID: "a", Fence: 3}
	instant := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	first := RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant)
	if first != RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant) || !strings.HasPrefix(first, "rej_") {
		t.Fatalf("id = %s", first)
	}
	if first == RejectionID(identity, RejectWrongAttempt, []byte("{}"), instant) || first == RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant.Add(time.Nanosecond)) {
		t.Fatal("id ignores its inputs")
	}
}

func TestAdmissionConflictReporting(t *testing.T) {
	t.Parallel()
	if !(Admission{Kind: KindReconsider}).ReportsLiveConflict() || (Admission{Kind: KindStandard}).ReportsLiveConflict() {
		t.Fatal("only reconsiderations report a live conflict")
	}
}

func TestRecoveryDocumentsAndRequeue(t *testing.T) {
	t.Parallel()
	item := UnfinishedAttempt{AttemptID: "a", EpisodeID: "e", Status: AttemptCancelling, OwnerEpoch: "old"}
	terminal, err := item.RecoveryTerminal()
	if err != nil || string(terminal) != `{"previous_owner_epoch":"old","reason":"runtime_restart","status":"abandoned"}` {
		t.Fatalf("terminal = %s err=%v", terminal, err)
	}
	if !item.Canceling() || (UnfinishedAttempt{Status: AttemptRunning}).Canceling() {
		t.Fatal("canceling classification changed")
	}
	if !IsRequeued(LifecycleAdmitted) || !IsRequeued(LifecycleRunning) || IsRequeued(LifecycleConcluded) {
		t.Fatal("requeue classification changed")
	}
}

func TestSchedulerRules(t *testing.T) {
	t.Parallel()
	if CheckStillPending(1, "i") != nil || CheckStillPending(0, "i") == nil {
		t.Fatal("pending check changed")
	}
}

func TestRejectionIDIsPinnedForWholeSecondAndFractionalInstants(t *testing.T) {
	t.Parallel()
	identity := Identity{EpisodeID: "e", AttemptID: "a", Fence: 1}
	for name, test := range map[string]struct {
		at   time.Time
		want string
	}{
		"whole second": {time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC), "rej_151d5295a1b297ed669b5b3b0c68e26c8b94c55475de86f1ac6076568f7641ed"},
		"fractional":   {time.Date(2026, 8, 12, 12, 0, 0, 500_000_000, time.UTC), "rej_87cea6ff9879c8ab4ad11791c6b47330073a9edc0708a1c87af4df19ec0a5beb"},
	} {
		if got := RejectionID(identity, RejectStaleAttempt, []byte("{}"), test.at); got != test.want {
			t.Errorf("%s: id = %s, want %s", name, got, test.want)
		}
	}
}
