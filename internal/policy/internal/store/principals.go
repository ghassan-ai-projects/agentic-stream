package store

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (tx *Tx) LoadApprovalPrincipals(ctx context.Context, row domain.IntentRecord, approver, relay string) (string, []byte, error) {
	var entityID string
	if err := tx.tx.QueryRowContext(ctx, "SELECT entity_id FROM situations WHERE situation_id = ?", row.SituationID).Scan(&entityID); err != nil {
		return "", nil, fmt.Errorf("load approval entity: %w", err)
	}
	var active int
	if err := tx.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", relay, row.TenantID).Scan(&active); err != nil || active != 1 {
		return "", nil, fmt.Errorf("relay principal is not active")
	}
	var publicKey []byte
	if err := tx.tx.QueryRowContext(ctx, "SELECT public_key FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", approver, row.TenantID).Scan(&publicKey); err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", nil, fmt.Errorf("approver principal has no valid verification key")
	}
	return entityID, publicKey, nil
}

func (tx *Tx) RequireApprovalAuthority(ctx context.Context, row domain.IntentRecord, entityID, approver string) error {
	var active int
	if err := tx.tx.QueryRowContext(ctx, approvalAuthoritySQL, approver, row.TenantID, row.TenantID, entityID, row.RiskClass).Scan(&active); err != nil || active == 0 {
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
