package domain

// LifecycleStatus is the coordination state of an episode aggregate. It does
// not describe a Decision, intent, command, or outcome.
type LifecycleStatus string

// Episode lifecycle states.
const (
	LifecycleAdmitted   LifecycleStatus = "admitted"
	LifecycleRunning    LifecycleStatus = "running"
	LifecycleConcluded  LifecycleStatus = "concluded"
	LifecycleClosed     LifecycleStatus = "closed"
	LifecycleSuperseded LifecycleStatus = "superseded"
	LifecycleExpired    LifecycleStatus = "expired"
	LifecycleAbandoned  LifecycleStatus = "abandoned"
)

// Closed reports whether the episode lifecycle is terminal.
func (s LifecycleStatus) Closed() bool {
	switch s {
	case LifecycleConcluded, LifecycleClosed, LifecycleSuperseded, LifecycleExpired, LifecycleAbandoned:
		return true
	default:
		return false
	}
}

// AttemptStatus is the state of one worker dispatch.
type AttemptStatus string

// Attempt states. The two spellings below are frozen durable protocol values.
const (
	AttemptDispatched AttemptStatus = "dispatched"
	AttemptRunning    AttemptStatus = "running"
	AttemptCancelling AttemptStatus = "cancelling" //nolint:misspell // Frozen durable protocol value.
	AttemptProduced   AttemptStatus = "produced"
	AttemptDeclined   AttemptStatus = "declined"
	AttemptCancelled  AttemptStatus = "cancelled" //nolint:misspell // Frozen durable protocol value.
	AttemptFailed     AttemptStatus = "failed"
	AttemptTimedOut   AttemptStatus = "timed_out"
	AttemptAbandoned  AttemptStatus = "abandoned"
)

// CanTransitionAttempt reports whether an attempt state transition is valid.
func CanTransitionAttempt(from, to AttemptStatus) bool {
	switch from {
	case AttemptDispatched:
		return to == AttemptRunning || to == AttemptCancelling || to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	case AttemptRunning:
		return to == AttemptCancelling || to == AttemptProduced || to == AttemptDeclined || to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	case AttemptCancelling:
		return to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	default:
		return false
	}
}

// IsTerminalAttempt reports whether an attempt state is terminal.
func IsTerminalAttempt(status AttemptStatus) bool {
	switch status {
	case AttemptProduced, AttemptDeclined, AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned:
		return true
	default:
		return false
	}
}
