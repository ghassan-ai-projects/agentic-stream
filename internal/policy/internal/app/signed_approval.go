package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func CheckApprovalSigner(ctx context.Context, tx *store.Tx, signed domain.SignedApproval) error {
	if _, err := signerKey(ctx, tx, signed); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrApprovalUnauthorized, err)
	}
	return nil
}

func VerifySignedApproval(ctx context.Context, tx *store.Tx, signed domain.SignedApproval) (string, error) {
	key, err := signerKey(ctx, tx, signed)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrApprovalUnauthorized, err)
	}
	digest, err := domain.VerifyAssertion(key, signed.Message, signed.Signature)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrApprovalUnauthorized, err)
	}
	return canonicaljson.EncodeDigest(digest), nil
}

func signerKey(ctx context.Context, tx *store.Tx, signed domain.SignedApproval) ([]byte, error) {
	if err := domain.DistinctPrincipals(domain.ApprovalResolution{Approver: signed.Approver, Relay: signed.Relay}); err != nil {
		return nil, err
	}
	key, err := activeRelayAndKey(ctx, tx, signed.TenantID, signed.Relay, signed.Approver)
	if err != nil {
		return nil, err
	}
	authority, err := tx.RiskAuthority(ctx, signed.TenantID, signed.Approver, signed.RiskClass)
	return key, domain.AuthorizedApprover(authority, err != nil)
}
