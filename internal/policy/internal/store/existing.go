package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// BindExistingResult projects command or approval identity for an evaluated intent.
func (tx *Tx) BindExistingResult(ctx context.Context, row domain.IntentRecord, result *domain.Result) error {
	if row.PolicyStatus == "approved" {
		commandID, err := tx.ExistingCommandID(ctx, row.IntentID)
		if err != nil {
			return fmt.Errorf("load existing command id: %w", err)
		}
		result.CommandID = commandID
	}
	if row.PolicyStatus == "approval_required" {
		approvalID, err := tx.PendingApproval(ctx, row.IntentID)
		if err != nil {
			return fmt.Errorf("load pending approval id: %w", err)
		}
		result.ApprovalID = approvalID
	}
	return nil
}
