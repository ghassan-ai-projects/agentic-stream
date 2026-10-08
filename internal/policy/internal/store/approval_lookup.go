package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var ErrUnreadableApprovalExpiry = errors.New("approval expiry is unreadable")

func (tx *Tx) PendingApproval(ctx context.Context, intentID string) (string, error) {
	approval, _, err := approvalledger.PendingOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return "", fmt.Errorf("find pending approval: %w", err)
	}
	return approval.ID, nil
}

// PendingApprovalExpiry reads the unresolved approval's identity and expiry.
func (tx *Tx) PendingApprovalExpiry(ctx context.Context, intentID string) (string, time.Time, error) {
	approval, found, err := approvalledger.PendingOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("load pending approval for expiry: %w", err)
	}
	if !found {
		return "", time.Time{}, nil
	}
	expiresAt, err := kernel.ParseTime(approval.ExpiresAt)
	if err != nil {
		return approval.ID, time.Time{}, fmt.Errorf("parse pending approval %s expiry: %w: %w", approval.ID, ErrUnreadableApprovalExpiry, err)
	}
	return approval.ID, expiresAt, nil
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
func (tx *Tx) AssertionBinding(ctx context.Context, id string) (time.Time, string, error) {
	binding, err := approvalledger.PendingBinding(ctx, tx.tx, id)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("load assertion binding of approval %s: %w", id, err)
	}
	expiresAt, err := kernel.ParseTime(binding.ExpiresAt)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse assertion binding of approval %s expiry: %w", id, err)
	}
	return expiresAt, binding.Nonce, nil
}
