package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// ErrPublisherRequired means a withdrawal would be silent: every withdrawn
// approval must publish its notification.
var ErrPublisherRequired = errors.New("withdrawal publisher is required")

// WithdrawSuperseded withdraws pending approvals bound to an older Situation
// version. Each approval is withdrawn and then published, in order, in the
// caller's transaction; a failed publication fails the whole withdrawal.
func WithdrawSuperseded(ctx context.Context, tx *store.Tx, situationID string, replacementVersion int, now string, publish store.Publisher) error {
	if publish == nil {
		return ErrPublisherRequired
	}
	withdrawals, err := tx.SupersededApprovals(ctx, situationID, replacementVersion)
	if err != nil {
		return err
	}
	for _, withdrawal := range withdrawals {
		if err := withdrawAndPublish(ctx, tx, withdrawal, now, publish); err != nil {
			return err
		}
	}
	return nil
}

func withdrawAndPublish(ctx context.Context, tx *store.Tx, withdrawal domain.Withdrawal, now string, publish store.Publisher) error {
	if err := Withdraw(ctx, tx, withdrawal.ApprovalID, now); err != nil {
		return fmt.Errorf("withdraw superseded approval %s: %w", withdrawal.ApprovalID, err)
	}
	return tx.Publish(ctx, publish, withdrawal)
}
