package store

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (tx *Tx) LoadApproval(ctx context.Context, approvalID string) (domain.ApprovalRecord, error) {
	var approval domain.ApprovalRecord
	err := tx.tx.QueryRowContext(ctx, "SELECT intent_id, status, expires_at FROM approvals WHERE approval_id = ?", approvalID).
		Scan(&approval.IntentID, &approval.Status, &approval.ExpiresAt)
	if err != nil {
		return approval, fmt.Errorf("load approval %s: %w", approvalID, err)
	}
	return approval, nil
}
