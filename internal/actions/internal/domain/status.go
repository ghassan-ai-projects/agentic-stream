package domain

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

// Outcome error codes recorded on the command outbox row.
const (
	ErrorOutcomeUnknown = "outcome_unknown"
	ErrorDispatchFailed = "dispatch_failed"
)
