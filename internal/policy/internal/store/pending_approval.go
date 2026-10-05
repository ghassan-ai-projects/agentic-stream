package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PendingApproval distinguishes a missing pending approval from a lookup failure.
func (tx *Tx) PendingApproval(ctx context.Context, intentID string) (string, error) {
	var approvalID string
	err := tx.tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", intentID).Scan(&approvalID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find pending approval: %w", err)
	}
	return approvalID, nil
}
