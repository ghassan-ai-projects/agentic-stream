package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func PendingOfIntent(ctx context.Context, tx *store.Tx, intentID string) (domain.Approval, bool, error) {
	return tx.PendingOfIntent(ctx, intentID)
}

func LatestApprovedOfIntent(ctx context.Context, tx *store.Tx, intentID string) (domain.Approval, bool, error) {
	return tx.LatestApprovedOfIntent(ctx, intentID)
}

func PendingBinding(ctx context.Context, tx *store.Tx, approvalID string) (domain.AssertionBinding, error) {
	return tx.PendingBinding(ctx, approvalID)
}
