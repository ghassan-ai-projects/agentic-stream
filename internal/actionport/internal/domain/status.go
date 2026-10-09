package domain

import "slices"

const (
	CommandPending        = "pending"
	CommandDispatching    = "dispatching"
	CommandSucceeded      = "succeeded"
	CommandFailed         = "failed"
	CommandReconciling    = "reconciling"
	CommandOutcomeUnknown = "outcome_unknown"
	CommandManualReview   = "manual_review"
)

const (
	VerificationObserved     = "observed"
	VerificationAwaiting     = "awaiting"
	VerificationReconciled   = "reconciled"
	VerificationRefuted      = "refuted"
	VerificationInconclusive = "inconclusive"
)

func UnresolvedCommandStatuses() []string {
	return []string{CommandOutcomeUnknown, CommandReconciling, CommandManualReview}
}

func IsUnresolvedCommandStatus(status string) bool {
	return slices.Contains(UnresolvedCommandStatuses(), status)
}
