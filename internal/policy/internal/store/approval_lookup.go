package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// PendingApproval distinguishes a missing pending approval from a lookup failure.
func (tx *Tx) PendingApproval(ctx context.Context, intentID string) (string, error) {
	approval, _, err := approvalledger.PendingOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return "", fmt.Errorf("find pending approval: %w", err)
	}
	return approval.ID, nil
}

// PendingApprovalExpiry reads the unresolved approval's identity and expiry.
func (tx *Tx) PendingApprovalExpiry(ctx context.Context, intentID string) (string, string, error) {
	approval, _, err := approvalledger.PendingOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return "", "", fmt.Errorf("load pending approval for expiry: %w", err)
	}
	return approval.ID, approval.ExpiresAt, nil
}

// ApprovedApproval reads the latest resolved human approval.
func (tx *Tx) ApprovedApproval(ctx context.Context, intentID string) (string, error) {
	approval, _, err := approvalledger.LatestApprovedOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return "", fmt.Errorf("load approved approval: %w", err)
	}
	return approval.ID, nil
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
	binding, err := approvalledger.PendingBinding(ctx, tx.tx, id)
	if err != nil {
		return "", "", fmt.Errorf("load assertion binding of approval %s: %w", id, err)
	}
	return binding.ExpiresAt, binding.Nonce, nil
}
