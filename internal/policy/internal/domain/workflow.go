package domain

import (
	"encoding/json"
	"time"
)

// CommandRecord is the sealed command and its durable identity.
type CommandRecord struct {
	ID             string
	JSON, SHA, Key []byte
	Target         string
}

// ApprovalRecord is a read-only projection of the approval lifecycle.
type ApprovalRecord struct {
	IntentID, Status, ExpiresAt string
	JSON                        json.RawMessage
}

// EvaluationRequest identifies the intent and evaluation time.
type EvaluationRequest struct {
	IntentID string
	Now      time.Time
}

// ApprovalResolution carries a human decision and its signed principals.
type ApprovalResolution struct {
	TenantID        string
	ID              string
	Approved        bool
	Approver, Relay string
	Signature       []byte
	Reason          string
	Now             time.Time
}

// ApprovalRequest is a sealed request and its notification payload.
type ApprovalRequest struct {
	ID, Nonce string
	Data      map[string]any
	JSON      []byte
}

// ApprovalContext contains the evidence projection used in an approval notice.
type ApprovalContext struct {
	Snapshot []byte
	Delta    map[string]any
	Decision DecisionDocument
	Source   string
}

// EvaluationAudit is the immutable policy evaluation evidence.
type EvaluationAudit struct {
	ID, PolicyVersion, PolicyDigest string
	Row                             IntentRecord
	Result                          Result
	Reason                          string
	Now                             time.Time
}

// CommandPreparation binds a command identity to one evaluated intent.
type CommandPreparation struct {
	ID, PolicyDigest string
	Row              IntentRecord
	Intent           IntentDocument
	Now              time.Time
}

// ApprovalNotice binds an approval request to its immutable evidence context.
type ApprovalNotice struct {
	Row       IntentRecord
	ID        string
	ExpiresAt time.Time
	Intent    IntentDocument
	Context   ApprovalContext
}

// Outcome separates the stable caller reason from detailed audit evidence.
type Outcome struct{ Status, Reason, AuditDetail string }

// AuditReason preserves the stable reason when there is no additional detail.
func (o Outcome) AuditReason() string {
	if o.AuditDetail != "" {
		return o.AuditDetail
	}
	return o.Reason
}

// IntentStatusChange carries policy's owned columns and operation context.
type IntentStatusChange struct {
	IntentID, Status, Operation string
	Now                         time.Time
}

// ApprovalPublication carries a sealed request and its durable validity interval.
type ApprovalPublication struct {
	IntentID       string
	Request        ApprovalRequest
	ExpiresAt, Now time.Time
}

// ApprovalEvent carries the lifecycle evidence of one approval disposition.
type ApprovalEvent struct {
	Intent             IntentRecord
	ID, Status, Reason string
	Now                time.Time
}
