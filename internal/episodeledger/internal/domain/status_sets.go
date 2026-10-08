package domain

import "strings"

var lifecycleStatuses = []LifecycleStatus{
	LifecycleAdmitted, LifecycleRunning, LifecycleConcluded, LifecycleClosed,
	LifecycleSuperseded, LifecycleExpired, LifecycleAbandoned,
}

var attemptStatuses = []AttemptStatus{
	AttemptDispatched, AttemptRunning, AttemptCancelling, AttemptProduced, AttemptDeclined,
	AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned,
}

func (s LifecycleStatus) Live() bool {
	return s == LifecycleAdmitted || s == LifecycleRunning
}

func (s LifecycleStatus) ProducedDecision() bool {
	return s == LifecycleConcluded || s == LifecycleClosed
}

func (s AttemptStatus) InFlight() bool {
	return s == AttemptDispatched || s == AttemptRunning
}

func (s AttemptStatus) Unfinished() bool {
	return s.InFlight() || s == AttemptCancelling
}

func (s AttemptStatus) CountsAsFailure() bool {
	return s == AttemptFailed || s == AttemptTimedOut || s == AttemptCancelled
}

func LifecycleSQL(member func(LifecycleStatus) bool) string {
	return sqlList(lifecycleStatuses, member)
}

func AttemptSQL(member func(AttemptStatus) bool) string {
	return sqlList(attemptStatuses, member)
}

func sqlList[S ~string](all []S, member func(S) bool) string {
	var quoted []string
	for _, status := range all {
		if member(status) {
			quoted = append(quoted, "'"+string(status)+"'")
		}
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}
