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

// Live reports whether the episode may still start or run an attempt.
func (s LifecycleStatus) Live() bool {
	return s == LifecycleAdmitted || s == LifecycleRunning
}

// ProducedDecision reports whether the episode concluded with a Decision that
// governance and dispatch may act on.
func (s LifecycleStatus) ProducedDecision() bool {
	return s == LifecycleConcluded || s == LifecycleClosed
}

// InFlight reports whether the attempt still accepts worker input: it has been
// dispatched and nobody has asked it to cancel. An attempt already asked to
// cancel is unfinished but not in flight.
func (s AttemptStatus) InFlight() bool {
	return s == AttemptDispatched || s == AttemptRunning
}

// Unfinished reports whether the attempt has not reached a terminal state.
func (s AttemptStatus) Unfinished() bool {
	return s.InFlight() || s == AttemptCancelling
}

// CountsAsFailure reports whether the attempt consumes the retry budget.
func (s AttemptStatus) CountsAsFailure() bool {
	return s == AttemptFailed || s == AttemptTimedOut || s == AttemptCancelled
}

// LifecycleSQL renders the lifecycle statuses accepted by member as a SQL
// value list, so a query selects exactly what the Go predicate selects.
func LifecycleSQL(member func(LifecycleStatus) bool) string {
	return sqlList(lifecycleStatuses, member)
}

// AttemptSQL renders the attempt statuses accepted by member as a SQL value
// list, so a query selects exactly what the Go predicate selects.
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
