package policy

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
)

func (g *Gateway) authorizeApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string, signature []byte) error {
	if approver == "" || relay == "" || approver == relay {
		return fmt.Errorf("relay and approver must be distinct registered principals")
	}
	entityID, key, err := loadApprovalPrincipals(ctx, tx, row, approver, relay)
	if err != nil {
		return err
	}
	if err := verifyApprovalAssertion(ctx, tx, row, approvalID, approver, relay, key, signature); err != nil {
		return err
	}
	return requireApprovalAuthority(ctx, tx, row, entityID, approver)
}

func loadApprovalPrincipals(ctx context.Context, tx *sql.Tx, row intentRow, approver, relay string) (string, []byte, error) {
	var entityID string
	if err := tx.QueryRowContext(ctx, "SELECT entity_id FROM situations WHERE situation_id = ?", row.SituationID).Scan(&entityID); err != nil {
		return "", nil, fmt.Errorf("load approval entity: %w", err)
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", relay, row.TenantID).Scan(&active); err != nil || active != 1 {
		return "", nil, fmt.Errorf("relay principal is not active")
	}
	var publicKey []byte
	if err := tx.QueryRowContext(ctx, "SELECT public_key FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", approver, row.TenantID).Scan(&publicKey); err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", nil, fmt.Errorf("approver principal has no valid verification key")
	}
	return entityID, publicKey, nil
}
func verifyApprovalAssertion(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string, publicKey, signature []byte) error {
	assertion, err := approvalSigningBytes(ctx, tx, row, approvalID, approver, relay)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), assertion, signature) {
		return fmt.Errorf("approval assertion signature is invalid")
	}
	assertionDigest := sha256.Sum256(assertion)
	if err := approvalledger.BindAssertion(ctx, tx, approvalID, assertionDigest[:]); err != nil {
		return fmt.Errorf("record approval assertion: %w", err)
	}
	return nil
}
func approvalSigningBytes(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string) ([]byte, error) {
	var expiresAt, nonce string
	if err := tx.QueryRowContext(ctx, "SELECT expires_at, nonce FROM approvals WHERE approval_id = ? AND status = 'pending'", approvalID).Scan(&expiresAt, &nonce); err != nil {
		return nil, fmt.Errorf("load approval assertion binding: %w", err)
	}
	assertion, err := ApprovalAssertionSigningBytes(ApprovalAssertion{
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
func requireApprovalAuthority(ctx context.Context, tx *sql.Tx, row intentRow, entityID, approver string) error {
	var active int
	if err := tx.QueryRowContext(ctx, approvalAuthoritySQL, approver, row.TenantID, row.TenantID, entityID, row.RiskClass).Scan(&active); err != nil || active == 0 {
		return fmt.Errorf("approver principal lacks authority")
	}
	return nil
}

const approvalAuthoritySQL = `
 SELECT COUNT(*) FROM principals p
 JOIN principal_roles pr ON pr.principal_id = p.principal_id
 JOIN approval_authorities aa ON aa.role_id = pr.role_id
 WHERE p.principal_id = ? AND p.tenant_id = ? AND p.status = 'active'
   AND aa.tenant_id = ? AND aa.entity_id = ? AND aa.risk_class = ?`
