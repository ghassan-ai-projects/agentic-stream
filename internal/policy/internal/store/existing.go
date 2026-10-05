package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (tx *Tx) BindExistingResult(ctx context.Context, row domain.IntentRecord, result *domain.Result) error {
	if row.PolicyStatus == "approved" {
		if err := tx.tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&result.CommandID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load existing command id: %w", err)
		}
	}
	if row.PolicyStatus == "approval_required" {
		if err := tx.tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&result.ApprovalID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load pending approval id: %w", err)
		}
	}
	return nil
}
