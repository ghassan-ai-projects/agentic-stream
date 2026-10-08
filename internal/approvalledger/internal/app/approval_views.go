package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func Approvals(ctx context.Context, reader store.Reader, intentID string) ([]domain.ApprovalView, error) {
	approvals, err := reader.Approvals(ctx, intentID)
	if err != nil {
		return nil, fmt.Errorf("approvals of intent %s: %w", intentID, err)
	}
	return approvals, nil
}
