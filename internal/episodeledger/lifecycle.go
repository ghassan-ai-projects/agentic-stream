package episodeledger

import (
	"errors"
	"fmt"
)

// LifecycleStatus is the coordination state of an episode aggregate. It does
// not describe a Decision, intent, command, or outcome.
type LifecycleStatus string

const (
	LifecycleAdmitted   LifecycleStatus = "admitted"
	LifecycleRunning    LifecycleStatus = "running"
	LifecycleConcluded  LifecycleStatus = "concluded"
	LifecycleClosed     LifecycleStatus = "closed"
	LifecycleSuperseded LifecycleStatus = "superseded"
	LifecycleExpired    LifecycleStatus = "expired"
	LifecycleAbandoned  LifecycleStatus = "abandoned"
)

// AttemptStatus is the terminal state of one worker dispatch.
type AttemptStatus string

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

// RejectionReason is a durable reason for refusing worker input or a
// proposed Decision.
type RejectionReason string

const (
	RejectUnknownEpisode       RejectionReason = "unknown_episode"
	RejectStaleAttempt         RejectionReason = "stale_attempt"
	RejectWrongAttempt         RejectionReason = "wrong_attempt"
	RejectTerminalAttempt      RejectionReason = "terminal_attempt"
	RejectEpisodeClosed        RejectionReason = "episode_closed"
	RejectSchemaInvalid        RejectionReason = "schema_invalid"
	RejectSnapshotMismatch     RejectionReason = "snapshot_mismatch"
	RejectEvidenceNotVisible   RejectionReason = "evidence_not_visible"
	RejectForgedReference      RejectionReason = "forged_reference"
	RejectOversized            RejectionReason = "oversized"
	RejectExpired              RejectionReason = "expired"
	RejectIntentTypeNotAllowed RejectionReason = "intent_type_not_allowed"
	RejectRiskCeilingExceeded  RejectionReason = "risk_ceiling_exceeded"
	// P4: the intent catalog is the independently-verified authority — these
	// reasons ride the same durable rejection registry.
	RejectCatalogMissing          RejectionReason = "catalog_missing"
	RejectCatalogForged           RejectionReason = "catalog_forged"
	RejectIntentTypeNotInCatalog  RejectionReason = "intent_type_not_in_catalog"
	RejectRiskLabelMismatch       RejectionReason = "risk_label_mismatch"
	RejectParameterSchemaViolated RejectionReason = "parameter_schema_violation"
	RejectPresetMismatch          RejectionReason = "preset_mismatch"
	RejectUngroundedEvidence      RejectionReason = "ungrounded_evidence"
)

// Identity is the fencing identity carried by every worker-produced object.
type Identity struct {
	EpisodeID  string
	AttemptID  string
	Fence      int64
	OwnerEpoch string
}

// IdentityError identifies why worker input was refused.
type IdentityError struct {
	Reason RejectionReason
}

func (e *IdentityError) Error() string {
	return fmt.Sprintf("worker identity rejected: %s", e.Reason)
}

// IsIdentityReason reports whether err is a fencing rejection with reason.
func IsIdentityReason(err error, reason RejectionReason) bool {
	var identityErr *IdentityError
	return errors.As(err, &identityErr) && identityErr.Reason == reason
}

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
