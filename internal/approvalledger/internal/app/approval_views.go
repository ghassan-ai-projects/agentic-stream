package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// Approvals reads every approval request of one intent, oldest first.
func Approvals(ctx context.Context, reader store.Reader, intentID string) ([]domain.ApprovalView, error) {
	return reader.Approvals(ctx, intentID) //nolint:wrapcheck // The store names the failed read.
}
