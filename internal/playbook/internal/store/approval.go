package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func (t *Tx) CheckSigner(ctx context.Context, tenantID, approver, relay, risk string) error {
	signed := policy.SignedApproval{TenantID: tenantID, Approver: approver, Relay: relay, RiskClass: risk}
	if err := policy.CheckApprovalSigner(ctx, t.tx, signed); err != nil {
		return fmt.Errorf("check the signer of a rule approval: %w", err)
	}
	return nil
}

func (t *Tx) VerifySignature(ctx context.Context, tenantID, approver, relay, risk string, message, signature []byte) (string, error) {
	signed := policy.SignedApproval{TenantID: tenantID, Approver: approver, Relay: relay, RiskClass: risk, Message: message, Signature: signature}
	digest, err := policy.VerifySignedApproval(ctx, t.tx, signed)
	if err != nil {
		return "", fmt.Errorf("verify a rule approval: %w", err)
	}
	return digest, nil
}
