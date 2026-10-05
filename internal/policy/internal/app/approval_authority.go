package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) authorizeApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, r domain.ApprovalResolution) error {
	if err := domain.DistinctPrincipals(r); err != nil {
		return err
	}
	entity, key, err := loadApprovalPrincipals(ctx, tx, row, r)
	if err != nil {
		return err
	}
	if err := verifyApprovalAssertion(ctx, tx, row, r, key); err != nil {
		return err
	}
	active, err := tx.ApprovalAuthority(ctx, row, entity, r.Approver)
	return domain.AuthorizedApprover(active, err != nil)
}
func loadApprovalPrincipals(ctx context.Context, tx *store.Tx, row domain.IntentRecord, r domain.ApprovalResolution) (string, []byte, error) {
	entity, err := tx.ApprovalEntity(ctx, row.SituationID)
	if err != nil {
		return "", nil, err
	}
	active, err := tx.RelayActivity(ctx, row.TenantID, r.Relay)
	if err := domain.ActiveRelay(active, err != nil); err != nil {
		return "", nil, err
	}
	key, err := tx.ApproverKey(ctx, row.TenantID, r.Approver)
	if err := domain.ValidApproverKey(key, err != nil); err != nil {
		return "", nil, err
	}
	return entity, key, nil
}
func verifyApprovalAssertion(ctx context.Context, tx *store.Tx, row domain.IntentRecord, r domain.ApprovalResolution, key []byte) error {
	assertion, err := approvalSigningBytes(ctx, tx, row, r)
	if err != nil {
		return err
	}
	digest, err := domain.VerifyAssertion(key, assertion, r.Signature)
	if err != nil {
		return err
	}
	return tx.BindAssertion(ctx, r.ID, digest)
}
func approvalSigningBytes(ctx context.Context, tx *store.Tx, row domain.IntentRecord, r domain.ApprovalResolution) ([]byte, error) {
	expires, nonce, err := tx.AssertionBinding(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	return domain.ApprovalAssertionSigningBytes(assertionFor(row, r, expires, nonce))
}
