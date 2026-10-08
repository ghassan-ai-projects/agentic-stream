package domain

import "testing"

func TestLifecycleStatusSetsCoverEveryValue(t *testing.T) {
	t.Parallel()
	want := map[LifecycleStatus]struct{ live, closed, producedDecision bool }{
		LifecycleAdmitted:   {live: true},
		LifecycleRunning:    {live: true},
		LifecycleConcluded:  {closed: true, producedDecision: true},
		LifecycleClosed:     {closed: true, producedDecision: true},
		LifecycleSuperseded: {closed: true},
		LifecycleExpired:    {closed: true},
		LifecycleAbandoned:  {closed: true},
	}
	if len(want) != len(lifecycleStatuses) {
		t.Fatalf("table covers %d of %d lifecycle statuses", len(want), len(lifecycleStatuses))
	}
	for _, status := range lifecycleStatuses {
		got := want[status]
		if status.Live() != got.live || status.Closed() != got.closed || status.ProducedDecision() != got.producedDecision {
			t.Errorf("%s: live=%v closed=%v producedDecision=%v", status, status.Live(), status.Closed(), status.ProducedDecision())
		}
		if status.Live() == status.Closed() {
			t.Errorf("%s is neither live nor closed, or both", status)
		}
	}
}

func TestAttemptStatusSetsCoverEveryValue(t *testing.T) {
	t.Parallel()
	want := map[AttemptStatus]struct{ inFlight, unfinished, failure bool }{
		AttemptDispatched: {inFlight: true, unfinished: true},
		AttemptRunning:    {inFlight: true, unfinished: true},
		AttemptCancelling: {unfinished: true},
		AttemptProduced:   {},
		AttemptDeclined:   {},
		AttemptCancelled:  {failure: true},
		AttemptFailed:     {failure: true},
		AttemptTimedOut:   {failure: true},
		AttemptAbandoned:  {},
	}
	if len(want) != len(attemptStatuses) {
		t.Fatalf("table covers %d of %d attempt statuses", len(want), len(attemptStatuses))
	}
	for _, status := range attemptStatuses {
		got := want[status]
		if status.InFlight() != got.inFlight || status.Unfinished() != got.unfinished || status.CountsAsFailure() != got.failure {
			t.Errorf("%s: inFlight=%v unfinished=%v failure=%v", status, status.InFlight(), status.Unfinished(), status.CountsAsFailure())
		}
		if IsTerminalAttempt(status) == status.Unfinished() {
			t.Errorf("%s: terminal and unfinished must be opposites", status)
		}
	}
}

func TestStatusSQLListsFollowThePredicates(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct{ got, want string }{
		"live lifecycles":      {LifecycleSQL(LifecycleStatus.Live), "('admitted', 'running')"},
		"closed lifecycles":    {LifecycleSQL(LifecycleStatus.Closed), "('concluded', 'closed', 'superseded', 'expired', 'abandoned')"},
		"decision lifecycles":  {LifecycleSQL(LifecycleStatus.ProducedDecision), "('concluded', 'closed')"},
		"in-flight attempts":   {AttemptSQL(AttemptStatus.InFlight), "('dispatched', 'running')"},
		"unfinished attempts":  {AttemptSQL(AttemptStatus.Unfinished), "('dispatched', 'running', 'cancelling')"},
		"failure attempts":     {AttemptSQL(AttemptStatus.CountsAsFailure), "('cancelled', 'failed', 'timed_out')"},
		"no lifecycle matches": {LifecycleSQL(func(LifecycleStatus) bool { return false }), "()"},
	} {
		if test.got != test.want {
			t.Errorf("%s = %s, want %s", name, test.got, test.want)
		}
	}
}
