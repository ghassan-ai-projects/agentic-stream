package domain

import "time"

// CommandRecord is the sealed command and its durable identity.
type CommandRecord struct {
	ID             string
	JSON, SHA, Key []byte
	Target         string
}

// ApprovalRecord is a read-only projection of the approval lifecycle.
type ApprovalRecord struct{ IntentID, Status, ExpiresAt string }

// EvaluationRequest identifies the intent and evaluation time.
type EvaluationRequest struct {
	IntentID string
	Now      time.Time
}

// ApprovalResolution carries a human decision and its signed principals.
type ApprovalResolution struct {
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
	Snapshot        []byte
	Delta, Decision map[string]any
	Source          string
}

// EvaluationAudit is the immutable policy evaluation evidence.
type EvaluationAudit struct {
	ID, PolicyVersion, PolicyDigest string
	Row                             IntentRecord
	Result                          Result
	Reason                          string
	Now                             time.Time
}
