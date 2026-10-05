package domain

import (
	"encoding/json"
	"errors"
)

// ErrApprovalNotFound hides absent and foreign-tenant approval identities.
var ErrApprovalNotFound = errors.New("approval not found")

// ErrApprovalUnauthorized refuses a submission without consuming its request.
var ErrApprovalUnauthorized = errors.New("approval not authorized")

// ErrApprovalResolved refuses signing a terminal request.
var ErrApprovalResolved = errors.New("approval already resolved")

// ApprovalLookup selects a tenant-bound request and the decision to sign.
type ApprovalLookup struct {
	ID, TenantID, Approver, Relay string
	Approved                      bool
}

// ApprovalPresentation contains the durable request and exact bytes to sign.
type ApprovalPresentation struct {
	ID           string          `json:"approval_id"`
	Status       string          `json:"status"`
	Request      json.RawMessage `json:"request"`
	SigningBytes []byte          `json:"signing_bytes"`
}
