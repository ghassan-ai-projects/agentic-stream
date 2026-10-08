package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// PendingOfIntent reads the unresolved approval of an intent.
func PendingOfIntent(ctx context.Context, tx *store.Tx, intentID string) (domain.Approval, bool, error) {
	return tx.PendingOfIntent(ctx, intentID)
}

// LatestApprovedOfIntent reads the most recently decided approved approval of an intent.
func LatestApprovedOfIntent(ctx context.Context, tx *store.Tx, intentID string) (domain.Approval, bool, error) {
	return tx.LatestApprovedOfIntent(ctx, intentID)
}

// PendingBinding reads the expiry and single-use nonce of a pending approval.
func PendingBinding(ctx context.Context, tx *store.Tx, approvalID string) (domain.AssertionBinding, error) {
	return tx.PendingBinding(ctx, approvalID)
}
