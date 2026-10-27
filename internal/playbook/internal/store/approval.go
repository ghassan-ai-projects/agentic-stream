package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNoSignatures = errors.New("rule approvals need the approval signature checks")

type SignedApproval struct {
	TenantID  string
	Approver  string
	Relay     string
	RiskClass string
	Message   []byte
	Signature []byte
}

type Signatures interface {
	CheckSigner(ctx context.Context, tx *sql.Tx, approval SignedApproval) error
	Verify(ctx context.Context, tx *sql.Tx, approval SignedApproval) (string, error)
}

func (t *Tx) CheckSigner(ctx context.Context, tenantID, approver, relay, risk string) error {
	if t.signatures == nil {
		return ErrNoSignatures
	}
	signed := SignedApproval{TenantID: tenantID, Approver: approver, Relay: relay, RiskClass: risk}
	if err := t.signatures.CheckSigner(ctx, t.tx, signed); err != nil {
		return fmt.Errorf("check the signer of a rule approval: %w", err)
	}
	return nil
}

func (t *Tx) VerifySignature(ctx context.Context, tenantID, approver, relay, risk string, message, signature []byte) (string, error) {
	if t.signatures == nil {
		return "", ErrNoSignatures
	}
	signed := SignedApproval{TenantID: tenantID, Approver: approver, Relay: relay, RiskClass: risk, Message: message, Signature: signature}
	digest, err := t.signatures.Verify(ctx, t.tx, signed)
	if err != nil {
		return "", fmt.Errorf("verify a rule approval: %w", err)
	}
	return digest, nil
}
