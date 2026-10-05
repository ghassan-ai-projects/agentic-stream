package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) authorizeApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID, approver, relay string, signature []byte) error {
	if approver == "" || relay == "" || approver == relay {
		return fmt.Errorf("relay and approver must be distinct registered principals")
	}
	entityID, key, err := tx.LoadApprovalPrincipals(ctx, row, approver, relay)
	if err != nil {
		return err
	}
	if err := verifyApprovalAssertion(ctx, tx, row, approvalID, approver, relay, key, signature); err != nil {
		return err
	}
	return tx.RequireApprovalAuthority(ctx, row, entityID, approver)
}

func verifyApprovalAssertion(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID, approver, relay string, publicKey, signature []byte) error {
	assertion, err := approvalSigningBytes(ctx, tx, row, approvalID, approver, relay)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), assertion, signature) {
		return fmt.Errorf("approval assertion signature is invalid")
	}
	assertionDigest := sha256.Sum256(assertion)
	if err := tx.BindAssertion(ctx, approvalID, assertionDigest[:]); err != nil {
		return err
	}
	return nil
}
func approvalSigningBytes(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID, approver, relay string) ([]byte, error) {
	expiresAt, nonce, err := tx.AssertionBinding(ctx, approvalID)
	if err != nil {
		return nil, err
	}
	assertion, err := domain.ApprovalAssertionSigningBytes(domain.ApprovalAssertion{
		ApprovalID: approvalID, IntentID: row.IntentID, DecisionID: row.DecisionID, TenantID: row.TenantID,
		SituationID: row.SituationID, SituationVersion: row.SituationVersion, RiskClass: row.RiskClass,
		IntentDigest: "sha256:" + hex.EncodeToString(row.IntentSHA), DecisionDigest: "sha256:" + hex.EncodeToString(row.DecisionSHA),
		ExpiresAt: expiresAt, Nonce: nonce, ApproverID: approver, RelayID: relay,
	})
	if err != nil {
		return nil, fmt.Errorf("approval assertion signature is invalid")
	}
	return assertion, nil
}
