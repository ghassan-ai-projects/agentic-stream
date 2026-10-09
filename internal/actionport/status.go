package actionport

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/actionport/internal/domain"
)

// Command ledger states (`commands.status`).
const (
	CommandPending        = domain.CommandPending
	CommandDispatching    = domain.CommandDispatching
	CommandSucceeded      = domain.CommandSucceeded
	CommandFailed         = domain.CommandFailed
	CommandReconciling    = domain.CommandReconciling
	CommandOutcomeUnknown = domain.CommandOutcomeUnknown
	CommandManualReview   = domain.CommandManualReview
)

// Verification states (`verifications.status`).
const (
	VerificationObserved     = domain.VerificationObserved
	VerificationAwaiting     = domain.VerificationAwaiting
	VerificationReconciled   = domain.VerificationReconciled
	VerificationRefuted      = domain.VerificationRefuted
	VerificationInconclusive = domain.VerificationInconclusive
)

// UnresolvedCommandStatuses returns the command states whose outcome still
// awaits reconciliation: the authority barrier, the operator list and the soak
// safety report all count exactly these.
func UnresolvedCommandStatuses() []string {
	return domain.UnresolvedCommandStatuses()
}

// IsUnresolvedCommandStatus reports whether status is one of
// UnresolvedCommandStatuses.
func IsUnresolvedCommandStatus(status string) bool {
	return domain.IsUnresolvedCommandStatus(status)
}
