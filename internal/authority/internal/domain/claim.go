package domain

import (
	"errors"
	"time"
)

// ErrTargetClaimBusy means another owner holds a live claim on the target.
var ErrTargetClaimBusy = errors.New("device target claim is held by another owner")

// ErrTargetClaimNotOwned means the caller is not the exact live holder of the
// claim it asserted or released.
var ErrTargetClaimNotOwned = errors.New("device target claim is not held by this owner")

// ClaimStatus is the recorded status of a held claim.
type ClaimStatus string

// Claim statuses.
const (
	ClaimActive   ClaimStatus = "active"
	ClaimReleased ClaimStatus = "released"
)

// HeldClaim is the claim currently recorded for a target, with its fence and
// lease.
type HeldClaim struct {
	TargetClaim
	Fence      int64
	LeaseUntil time.Time
	Status     ClaimStatus
}

// Live reports whether the claim is active and its lease has not expired.
func (h *HeldClaim) Live(now time.Time) bool {
	return h != nil && h.Status == ClaimActive && h.LeaseUntil.After(now)
}

func (h *HeldClaim) heldBy(owner Owner) bool {
	return h != nil && h.Owner == owner
}

func (h *HeldClaim) is(claim TargetClaim) bool {
	return h != nil && h.TargetClaim == claim
}

// ClaimDecision is the outcome of a claim attempt: the audited event and the
// fence the written claim carries.
type ClaimDecision struct {
	Event EventType
	Fence int64
}

// Rejected reports whether the attempt must be refused as busy.
func (d ClaimDecision) Rejected() bool {
	return d.Event == EventClaimRejected
}

// DecideClaim rejects a claim while another owner holds a live claim. Otherwise
// the claim is acquired, or renewed when the same owner's claim is still
// active, and the fence is kept only for a live renewal.
func DecideClaim(held *HeldClaim, claim TargetClaim, now time.Time) ClaimDecision {
	if held.Live(now) && !held.heldBy(claim.Owner) {
		return ClaimDecision{Event: EventClaimRejected}
	}
	event := EventClaimAcquired
	if held.heldBy(claim.Owner) && held.Status == ClaimActive {
		event = EventClaimRenewed
	}
	return ClaimDecision{Event: event, Fence: nextFence(held, claim.Owner, now)}
}

// nextFence keeps the fence for a live renewal by the same owner and advances
// it for every takeover, so a stale owner can never present a current fence.
func nextFence(held *HeldClaim, owner Owner, now time.Time) int64 {
	if held == nil {
		return 1
	}
	if held.heldBy(owner) && held.Live(now) {
		return held.Fence
	}
	return held.Fence + 1
}

// ClaimEvent is the audit record of a claim decision.
func ClaimEvent(decision ClaimDecision, claim TargetClaim, held *HeldClaim, now time.Time) AuthorityEvent {
	if decision.Rejected() {
		return newEvent(EventClaimRejected, claim, map[string]any{
			"reason": "unexpired_owner", "current_epoch": held.Owner.Epoch, "current_instance": held.Owner.Instance,
		}, now)
	}
	return newEvent(decision.Event, claim, map[string]any{"claim_fence": decision.Fence}, now)
}

// CheckClaimHeld requires the exact claim to be live.
func CheckClaimHeld(held *HeldClaim, claim TargetClaim, now time.Time) error {
	if !held.is(claim) || !held.Live(now) {
		return ErrTargetClaimNotOwned
	}
	return nil
}

// CheckReleasable requires the exact claim to be active. An expired but
// active claim may still be released by its holder.
func CheckReleasable(held *HeldClaim, claim TargetClaim) error {
	if !held.is(claim) || held.Status != ClaimActive {
		return ErrTargetClaimNotOwned
	}
	return nil
}

// ReleaseEvent is the audit record of a released claim.
func ReleaseEvent(claim TargetClaim, now time.Time) AuthorityEvent {
	return newEvent(EventClaimReleased, claim, nil, now)
}
