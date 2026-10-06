package domain

import "context"

// Readiness reports whether the runtime can safely accept work.
type Readiness interface {
	Ready() error
}

// Problem is RFC 9457 Problem Details for HTTP API failures.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ApprovalSelection identifies a request and the human decision to sign.
type ApprovalSelection struct {
	ID, Approver, Relay string
	Approved            bool
}

// ApprovalSubmission carries a signed decision; tenant and time belong to runtime.
type ApprovalSubmission struct {
	ApprovalSelection
	Signature []byte
	Reason    string
}

// ApprovalConfig binds transport callbacks to one authenticated relay.
type ApprovalConfig struct {
	Present      func(context.Context, ApprovalSelection) (any, error)
	Resolve      func(context.Context, ApprovalSubmission) (any, error)
	ErrorStatus  func(error) int
	Token, Relay string
}

// Configured reports whether every callback and credential is present.
func (c ApprovalConfig) Configured() bool {
	return c.Present != nil && c.Resolve != nil && c.ErrorStatus != nil && c.Token != "" && c.Relay != ""
}
