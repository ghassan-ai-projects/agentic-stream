package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// ApprovalEntity projects the Situation's durable entity binding.
func (tx *Tx) ApprovalEntity(ctx context.Context, situationID string) (string, error) {
	var entity string
	if err := tx.tx.QueryRowContext(ctx, "SELECT entity_id FROM situations WHERE situation_id = ?", situationID).Scan(&entity); err != nil {
		return "", fmt.Errorf("load approval entity: %w", err)
	}
	return entity, nil
}

// RelayActivity reads the active relay count for the approval's tenant.
func (tx *Tx) RelayActivity(ctx context.Context, tenant, relay string) (int, error) {
	var active int
	if err := tx.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", relay, tenant).Scan(&active); err != nil {
		return 0, fmt.Errorf("load relay principal: %w", err)
	}
	return active, nil
}

// ApproverKey reads the active approver's public verification key.
func (tx *Tx) ApproverKey(ctx context.Context, tenant, approver string) ([]byte, error) {
	var key []byte
	if err := tx.tx.QueryRowContext(ctx, "SELECT public_key FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", approver, tenant).Scan(&key); err != nil {
		return nil, fmt.Errorf("load approver principal: %w", err)
	}
	return key, nil
}

// ApprovalAuthority reads the matching role authority count.
func (tx *Tx) ApprovalAuthority(ctx context.Context, row domain.IntentRecord, entity, approver string) (int, error) {
	var active int
	if err := tx.tx.QueryRowContext(ctx, approvalAuthoritySQL, approver, row.TenantID, row.TenantID, entity, row.RiskClass).Scan(&active); err != nil {
		return 0, fmt.Errorf("load approval authority: %w", err)
	}
	return active, nil
}

const approvalAuthoritySQL = `
 SELECT COUNT(*) FROM principals p
 JOIN principal_roles pr ON pr.principal_id = p.principal_id
 JOIN approval_authorities aa ON aa.role_id = pr.role_id
 WHERE p.principal_id = ? AND p.tenant_id = ? AND p.status = 'active'
   AND aa.tenant_id = ? AND aa.entity_id = ? AND aa.risk_class = ?`
