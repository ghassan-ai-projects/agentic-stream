package domain

// Approval states. Every transition starts from pending and is a no-op for an
// approval that already left it.
const (
	StatusPending = "pending"
	StatusExpired = "expired"
	StatusDenied  = "denied"

	StatusApproved = "approved"
)

// Stable reasons recorded with a transition.
const (
	ReasonExpired      = "approval_expired"
	ReasonWithdrawn    = "approval_withdrawn"
	WithdrawalConflict = "situation_version_conflict"
)

// Withdrawal is a pending approval withdrawn because a newer Situation version
// replaced the one it was bound to. It carries what a withdrawal notification
// needs, including the W3C trace context of the Decision that proposed it.
type Withdrawal struct {
	ApprovalID       string
	IntentID         string
	SituationID      string
	SituationVersion int
	Traceparent      string
	Tracestate       string
}

type Approval struct {
	ID        string
	ExpiresAt string
}

type AssertionBinding struct {
	ExpiresAt string
	Nonce     string
}
