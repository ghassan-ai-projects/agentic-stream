package domain

import (
	"slices"
	"strings"
	"testing"
)

var allowedAttemptTransitions = map[AttemptStatus][]AttemptStatus{
	AttemptDispatched: {AttemptRunning, AttemptCancelling, AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned},
	AttemptRunning:    {AttemptCancelling, AttemptProduced, AttemptDeclined, AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned},
	AttemptCancelling: {AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned},
}

func TestAnAttemptMovesOnlyAlongTheTransitionTable(t *testing.T) {
	t.Parallel()
	for _, from := range attemptStatuses {
		t.Run(string(from), func(t *testing.T) {
			t.Parallel()
			for _, to := range attemptStatuses {
				want := slices.Contains(allowedAttemptTransitions[from], to)
				if got := CanTransitionAttempt(from, to); got != want {
					t.Errorf("CanTransitionAttempt(%s, %s) = %v, want %v", from, to, got, want)
				}
				if got := CheckTransition(from, to) == nil; got != want {
					t.Errorf("CheckTransition(%s, %s) accepted = %v, want %v", from, to, got, want)
				}
			}
		})
	}
}

func TestTerminalAttemptsAcceptNoTransition(t *testing.T) {
	t.Parallel()
	for _, from := range attemptStatuses {
		if from.Unfinished() {
			continue
		}
		for _, to := range attemptStatuses {
			if CanTransitionAttempt(from, to) {
				t.Errorf("terminal attempt %s may move to %s", from, to)
			}
		}
	}
}

func TestARefusedTransitionNamesBothStates(t *testing.T) {
	t.Parallel()
	err := CheckTransition(AttemptProduced, AttemptRunning)
	if err == nil || !strings.Contains(err.Error(), "invalid attempt transition produced -> running") {
		t.Fatalf("CheckTransition(produced, running) = %v, want a message naming both states", err)
	}
}

func TestATransitionWritesTheColumnsOfItsKind(t *testing.T) {
	t.Parallel()
	want := map[AttemptStatus]TransitionKind{
		AttemptDispatched: TransitionStatusOnly,
		AttemptRunning:    TransitionRunning,
		AttemptCancelling: TransitionStatusOnly,
		AttemptProduced:   TransitionTerminal,
		AttemptDeclined:   TransitionTerminal,
		AttemptCancelled:  TransitionTerminal,
		AttemptFailed:     TransitionTerminal,
		AttemptTimedOut:   TransitionTerminal,
		AttemptAbandoned:  TransitionTerminal,
	}
	for _, status := range attemptStatuses {
		if got := KindOf(status); got != want[status] {
			t.Errorf("KindOf(%s) = %d, want %d", status, got, want[status])
		}
	}
}

func TestOnlyCancellationAndAbandonmentAcknowledgeACancelledEpisode(t *testing.T) {
	t.Parallel()
	for _, to := range attemptStatuses {
		want := to == AttemptCancelling || to == AttemptCancelled || to == AttemptAbandoned
		if got := MayAcknowledgeCancellation(to); got != want {
			t.Errorf("MayAcknowledgeCancellation(%s) = %v, want %v", to, got, want)
		}
	}
}
