package domain

// Command ledger states (`commands.status`).
const (
	CommandPending        = "pending"
	CommandDispatching    = "dispatching"
	CommandSucceeded      = "succeeded"
	CommandFailed         = "failed"
	CommandReconciling    = "reconciling"
	CommandOutcomeUnknown = "outcome_unknown"
	CommandManualReview   = "manual_review"
)

// Command outbox states (`outbox.status`).
const (
	OutboxPending   = "pending"
	OutboxLeased    = "leased"
	OutboxDelivered = "delivered"
	OutboxFailed    = "failed"
)

// Outcome states (`outcomes.status`).
const (
	OutcomeSucceeded         = "succeeded"
	OutcomeFailed            = "failed"
	OutcomeUnknown           = "unknown"
	OutcomeReconcileRequired = "reconcile_required"
	OutcomeReconciled        = "reconciled"
)

// Outcome reconciliation states (`outcomes.reconciliation_status`).
const (
	ReconciliationObserved    = "observed"
	ReconciliationRequired    = "required"
	ReconciliationNotRequired = "not_required"
	ReconciliationReconciled  = "reconciled"
)

// Verification states (`verifications.status`).
const (
	VerificationObserved     = "observed"
	VerificationAwaiting     = "awaiting"
	VerificationReconciled   = "reconciled"
	VerificationRefuted      = "refuted"
	VerificationInconclusive = "inconclusive"
)

// Outcome error codes recorded on the command outbox row.
const (
	ErrorOutcomeUnknown = "outcome_unknown"
	ErrorDispatchFailed = "dispatch_failed"
)

// UnresolvedCommandStatuses are the command states whose outcome still awaits
// reconciliation.
var UnresolvedCommandStatuses = []string{CommandOutcomeUnknown, CommandReconciling, CommandManualReview}
