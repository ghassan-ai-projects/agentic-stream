package domain

// ApprovalRequested asks a human to approve an intent, with the evidence the
// approver sees. Delta and Action must be non-nil objects.
type ApprovalRequested struct {
	ApprovalID         string         `json:"approval_id"`
	IntentID           string         `json:"intent_id"`
	DecisionID         string         `json:"decision_id"`
	SituationID        string         `json:"situation_id"`
	SituationVersion   int            `json:"situation_version"`
	IntentDigest       string         `json:"intent_digest"`
	SnapshotDigest     string         `json:"snapshot_digest"`
	RiskClass          string         `json:"risk_class"`
	ExpiresAt          string         `json:"expires_at"`
	Audience           string         `json:"audience"`
	Summary            string         `json:"summary"`
	Delta              map[string]any `json:"delta"`
	Hypothesis         string         `json:"hypothesis"`
	Evidence           []string       `json:"evidence"`
	Action             map[string]any `json:"action"`
	DeclineConsequence string         `json:"decline_consequence"`
}

// EventType implements Payload.
func (ApprovalRequested) EventType() string { return TypeApprovalRequested }

// ApprovalWithdrawn records that a pending approval was withdrawn.
type ApprovalWithdrawn struct {
	ApprovalID       string `json:"approval_id"`
	IntentID         string `json:"intent_id"`
	SituationID      string `json:"situation_id"`
	SituationVersion int    `json:"situation_version"`
	Reason           string `json:"reason"`
}

// EventType implements Payload.
func (ApprovalWithdrawn) EventType() string { return TypeApprovalWithdrawn }

// ApprovalResolved records the disposition of an approval.
type ApprovalResolved struct {
	ApprovalID       string `json:"approval_id"`
	IntentID         string `json:"intent_id"`
	DecisionID       string `json:"decision_id"`
	SituationID      string `json:"situation_id"`
	SituationVersion int    `json:"situation_version"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
}

// EventType implements Payload.
func (ApprovalResolved) EventType() string { return TypeApprovalResolved }
