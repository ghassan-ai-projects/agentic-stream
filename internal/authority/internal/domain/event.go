package domain

import "time"

// EventType names one kind of authority event in the append-only audit log.
type EventType string

// Authority event types. Safe-stop stages are event types as well.
const (
	EventClaimAcquired          EventType = "claim_acquired"
	EventClaimRenewed           EventType = "claim_renewed"
	EventClaimRejected          EventType = "claim_rejected"
	EventClaimReleased          EventType = "claim_released"
	EventReconciliationOpened   EventType = "reconciliation_opened"
	EventReconciliationRecorded EventType = "reconciliation_recorded"
)

// AuthorityEvent is one audited claim, reconciliation or safe-stop transition.
type AuthorityEvent struct {
	Type       EventType
	Subject    TargetClaim
	Details    map[string]any
	OccurredAt time.Time
}

func newEvent(eventType EventType, subject TargetClaim, details map[string]any, at time.Time) AuthorityEvent {
	if details == nil {
		details = map[string]any{}
	}
	return AuthorityEvent{Type: eventType, Subject: subject, Details: details, OccurredAt: at}
}
