package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// PendingApprovalExpiry reads the unresolved approval's identity and expiry.
func (tx *Tx) PendingApprovalExpiry(ctx context.Context, intentID string) (string, string, error) {
	var id, expiry string
	err := tx.tx.QueryRowContext(ctx, "SELECT approval_id, expires_at FROM approvals WHERE intent_id = ? AND status = 'pending'", intentID).Scan(&id, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load pending approval for expiry: %w", err)
	}
	return id, expiry, nil
}

// ApprovedApproval reads the latest resolved human approval.
func (tx *Tx) ApprovedApproval(ctx context.Context, intentID string) (string, error) {
	id, _, err := storage.QueryOptional[string](ctx, tx.tx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'approved' ORDER BY decided_at DESC LIMIT 1", intentID)
	if err != nil {
		return "", fmt.Errorf("load approved approval: %w", err)
	}
	return id, nil
}

// CompensationTenant projects command ownership for compensation validation.
func (tx *Tx) CompensationTenant(ctx context.Context, commandID string) (string, bool, error) {
	tenant, found, err := storage.QueryOptional[string](ctx, tx.tx, "SELECT tenant_id FROM commands WHERE command_id = ?", commandID)
	if err != nil {
		return "", false, fmt.Errorf("load compensation target: %w", err)
	}
	return tenant, found, nil
}

// AssertionBinding reads the durable single-use assertion identity.
func (tx *Tx) AssertionBinding(ctx context.Context, id string) (string, string, error) {
	var expiry, nonce string
	if err := tx.tx.QueryRowContext(ctx, "SELECT expires_at, nonce FROM approvals WHERE approval_id = ? AND status = 'pending'", id).Scan(&expiry, &nonce); err != nil {
		return "", "", fmt.Errorf("load approval assertion binding: %w", err)
	}
	return expiry, nonce, nil
}
