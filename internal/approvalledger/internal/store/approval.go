package store

import (
	"context"
	"fmt"
)

// InsertPending records a digest-bound approval request as pending.
func (t *Tx) InsertPending(ctx context.Context, approvalID, intentID, requestedAt, expiresAt string, document []byte, nonce string) error {
	return t.write(ctx, `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, approval_json, nonce) VALUES (?, ?, 'pending', ?, ?, ?, ?)`,
		approvalID, intentID, requestedAt, expiresAt, document, nonce)
}

// ExpirePendingOfIntent expires the pending approval of an intent.
func (t *Tx) ExpirePendingOfIntent(ctx context.Context, intentID string) error {
	return t.write(ctx, `UPDATE approvals SET status = 'expired' WHERE intent_id = ? AND status = 'pending'`, intentID)
}

// ExpirePending records the expiry of one still-pending approval.
func (t *Tx) ExpirePending(ctx context.Context, approvalID, now, reason string) error {
	return t.write(ctx, `UPDATE approvals SET status = 'expired', decided_at = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`, now, reason, approvalID)
}

// DecidePending records a principal decision on a still-pending approval.
func (t *Tx) DecidePending(ctx context.Context, approvalID, status, approver, relay, reason, now string) error {
	return t.write(ctx, `UPDATE approvals SET status = ?, decided_at = ?, approver_identity = ?, relay_identity = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`,
		status, now, approver, relay, reason, approvalID)
}

// BindAssertionDigest binds the verified assertion without changing the
// single-use nonce.
func (t *Tx) BindAssertionDigest(ctx context.Context, approvalID string, digest []byte) error {
	return t.write(ctx, `UPDATE approvals SET assertion_sha256 = ?, nonce = nonce WHERE approval_id = ? AND status = 'pending'`, digest, approvalID)
}

// WithdrawPending denies a still-pending approval and records why it was withdrawn.
func (t *Tx) WithdrawPending(ctx context.Context, approvalID, now, withdrawalReason, reason string) error {
	return t.write(ctx, `UPDATE approvals SET status = 'denied', decided_at = ?, withdrawn_at = ?, withdrawal_reason = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`,
		now, now, withdrawalReason, reason, approvalID)
}

func (t *Tx) write(ctx context.Context, query string, args ...any) error {
	if _, err := t.tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("write approval: %w", err)
	}
	return nil
}
