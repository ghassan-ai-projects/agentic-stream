package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// RequestApproval delegates lifecycle creation to the approval ledger.
func (tx *Tx) RequestApproval(ctx context.Context, p domain.ApprovalPublication) error {
	r := p.Request
	if err := approvalledger.Request(ctx, tx.tx, r.ID, p.IntentID, domain.FormatTime(p.Now), domain.FormatTime(p.ExpiresAt), r.JSON, r.Nonce); err != nil {
		return fmt.Errorf("insert approval: %w", err)
	}
	return nil
}

// ResolveApproval delegates a human decision to the approval ledger.
func (tx *Tx) ResolveApproval(ctx context.Context, r domain.ApprovalResolution, status, operation string) error {
	if err := approvalledger.Resolve(ctx, tx.tx, r.ID, status, r.Approver, r.Relay, r.Reason, domain.FormatTime(r.Now)); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// WithdrawApproval delegates stale withdrawal to the approval ledger.
func (tx *Tx) WithdrawApproval(ctx context.Context, id string, now time.Time) error {
	if err := approvalledger.Withdraw(ctx, tx.tx, id, domain.FormatTime(now)); err != nil {
		return fmt.Errorf("withdraw stale approval: %w", err)
	}
	return nil
}

// ExpireApproval delegates expiry to the approval ledger.
func (tx *Tx) ExpireApproval(ctx context.Context, id string, now time.Time) error {
	if err := approvalledger.Expire(ctx, tx.tx, id, domain.FormatTime(now)); err != nil {
		return fmt.Errorf("expire approval %s: %w", id, err)
	}
	return nil
}

// ExpireIntentApproval expires the pending approval for an intent.
func (tx *Tx) ExpireIntentApproval(ctx context.Context, intentID string) error {
	if err := approvalledger.ExpireIntent(ctx, tx.tx, intentID); err != nil {
		return fmt.Errorf("expire approval: %w", err)
	}
	return nil
}

// BindAssertion records the verified signature binding exactly once.
func (tx *Tx) BindAssertion(ctx context.Context, id string, digest []byte) error {
	if err := approvalledger.BindAssertion(ctx, tx.tx, id, digest); err != nil {
		return fmt.Errorf("record approval assertion: %w", err)
	}
	return nil
}
