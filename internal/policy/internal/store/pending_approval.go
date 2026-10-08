package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// PendingApproval distinguishes a missing pending approval from a lookup failure.
func (tx *Tx) PendingApproval(ctx context.Context, intentID string) (string, error) {
	approvalID, _, err := storage.QueryOptional[string](ctx, tx.tx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", intentID)
	if err != nil {
		return "", fmt.Errorf("find pending approval: %w", err)
	}
	return approvalID, nil
}
