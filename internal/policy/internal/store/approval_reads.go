package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// LoadApproval reads the durable human approval lifecycle projection.
func (tx *Tx) LoadApproval(ctx context.Context, approvalID, tenant string) (domain.ApprovalRecord, error) {
	var approval domain.ApprovalRecord
	var expiresAt string
	err := tx.tx.QueryRowContext(ctx, "SELECT a.intent_id, a.status, a.expires_at, a.approval_json FROM approvals a JOIN intents i ON i.intent_id = a.intent_id WHERE a.approval_id = ? AND i.tenant_id = ?", approvalID, tenant).
		Scan(&approval.IntentID, &approval.Status, &expiresAt, &approval.JSON)
	if errors.Is(err, sql.ErrNoRows) {
		return approval, domain.ErrApprovalNotFound
	}
	if err != nil {
		return approval, fmt.Errorf("load approval %s: %w", approvalID, err)
	}
	if approval.ExpiresAt, err = kernel.ParseTime(expiresAt); err != nil {
		return approval, fmt.Errorf("load approval %s expiry: %w", approvalID, err)
	}
	return approval, nil
}
