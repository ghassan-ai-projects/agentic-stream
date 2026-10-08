package domain

import (
	"errors"
	"fmt"
)

// ErrOwnerLost is what a runtime owner check reports when the epoch no longer
// owns the runtime; any other check error is a failure to check.
var ErrOwnerLost = errors.New("runtime owner lost")

// EpisodeFence is what the ledger records about an episode's current attempt.
type EpisodeFence struct {
	TenantID   string
	Lifecycle  LifecycleStatus
	Attempt    string
	HasAttempt bool
	Fence      int64
}

// AttemptRecord is what the ledger records about one attempt row.
type AttemptRecord struct {
	Status        AttemptStatus
	OwnerEpoch    string
	HasOwnerEpoch bool
}

// CheckOpenIdentity requires an open episode whose current attempt and fence
// are exactly the identity's; an older fence is stale.
func (f EpisodeFence) CheckOpenIdentity(identity Identity) error {
	if f.Lifecycle.Closed() {
		return Refuse(RejectEpisodeClosed)
	}
	return f.CheckIdentity(identity)
}

// CheckIdentity compares the identity with the recorded attempt and fence.
func (f EpisodeFence) CheckIdentity(identity Identity) error {
	if identity.Fence < f.Fence {
		return Refuse(RejectStaleAttempt)
	}
	if !f.HasAttempt || identity.AttemptID != f.Attempt || identity.Fence != f.Fence {
		return Refuse(RejectWrongAttempt)
	}
	return nil
}

// CheckStartable requires an admitted or running episode and returns its
// current fence; the caller still has to verify the prior attempt is terminal.
func (f EpisodeFence) CheckStartable() (int64, error) {
	if !f.Lifecycle.Live() {
		return 0, Refuse(RejectEpisodeClosed)
	}
	return f.Fence, nil
}

// CheckAttemptOpen requires the attempt row to exist under the identity's
// owner epoch and not be terminal.
func CheckAttemptOpen(identity Identity, record AttemptRecord) error {
	if identity.OwnerEpoch != "" && (!record.HasOwnerEpoch || record.OwnerEpoch != identity.OwnerEpoch) {
		return Refuse(RejectStaleAttempt)
	}
	return CheckAttemptNotTerminal(record.Status)
}

// CheckAttemptNotTerminal refuses a terminal attempt.
func CheckAttemptNotTerminal(status AttemptStatus) error {
	if IsTerminalAttempt(status) {
		return Refuse(RejectTerminalAttempt)
	}
	return nil
}

// CheckPriorAttemptTerminal refuses a new attempt while the previous one is
// still active.
func CheckPriorAttemptTerminal(episodeID, attemptID string, status AttemptStatus) error {
	if !IsTerminalAttempt(status) {
		return fmt.Errorf("episode %s already has active attempt %s", episodeID, attemptID)
	}
	return nil
}

// CheckTransition refuses an invalid attempt state transition.
func CheckTransition(from, to AttemptStatus) error {
	if !CanTransitionAttempt(from, to) {
		return fmt.Errorf("invalid attempt transition %s -> %s", from, to)
	}
	return nil
}

// MayAcknowledgeCancellation reports whether a transition may close an
// attempt of a superseded episode: only cancellation or abandonment, never a
// produced Decision.
func MayAcknowledgeCancellation(to AttemptStatus) bool {
	return to == AttemptCancelling || to == AttemptCancelled || to == AttemptAbandoned
}

// TransitionKind says which attempt columns a transition writes.
type TransitionKind int

// Transition kinds.
const (
	TransitionTerminal TransitionKind = iota
	TransitionRunning
	TransitionStatusOnly
)

// KindOf classifies a target attempt status.
func KindOf(to AttemptStatus) TransitionKind {
	if IsTerminalAttempt(to) {
		return TransitionTerminal
	}
	if to == AttemptRunning {
		return TransitionRunning
	}
	return TransitionStatusOnly
}
