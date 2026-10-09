package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
)

const pendingOfIntentSQL = `SELECT approval_id, expires_at FROM approvals WHERE intent_id = ? AND status = ?`

const latestApprovedOfIntentSQL = `SELECT approval_id, expires_at FROM approvals WHERE intent_id = ? AND status = ? ORDER BY decided_at DESC, approval_id DESC LIMIT 1`

const pendingBindingSQL = `SELECT expires_at, nonce FROM approvals WHERE approval_id = ? AND status = ?`

func (t *Tx) PendingOfIntent(ctx context.Context, intentID string) (domain.Approval, bool, error) {
	approval, found, err := t.lookup(ctx, pendingOfIntentSQL, intentID, domain.StatusPending)
	if err != nil {
		return domain.Approval{}, false, fmt.Errorf("load pending approval: %w", err)
	}
	return approval, found, nil
}

func (t *Tx) LatestApprovedOfIntent(ctx context.Context, intentID string) (domain.Approval, bool, error) {
	approval, found, err := t.lookup(ctx, latestApprovedOfIntentSQL, intentID, domain.StatusApproved)
	if err != nil {
		return domain.Approval{}, false, fmt.Errorf("load approved approval: %w", err)
	}
	return approval, found, nil
}

func (t *Tx) PendingBinding(ctx context.Context, approvalID string) (domain.AssertionBinding, error) {
	var binding domain.AssertionBinding
	if err := t.tx.QueryRowContext(ctx, pendingBindingSQL, approvalID, domain.StatusPending).Scan(&binding.ExpiresAt, &binding.Nonce); err != nil {
		return domain.AssertionBinding{}, fmt.Errorf("load approval assertion binding: %w", err)
	}
	return binding, nil
}

func (t *Tx) lookup(ctx context.Context, query, key, status string) (domain.Approval, bool, error) {
	var approval domain.Approval
	err := t.tx.QueryRowContext(ctx, query, key, status).Scan(&approval.ID, &approval.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Approval{}, false, nil
	}
	if err != nil {
		return domain.Approval{}, false, fmt.Errorf("scan approval: %w", err)
	}
	return approval, true, nil
}
