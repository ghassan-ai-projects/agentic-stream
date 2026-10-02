// Package approvalledger owns the durable human approval lifecycle.
package approvalledger

import (
	"context"
	"database/sql"
	"fmt"
)

// Request records the digest-bound human approval request.
func Request(ctx context.Context, tx *sql.Tx, approvalID, intentID, requestedAt, expiresAt string, document []byte, nonce string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, approval_json, nonce) VALUES (?, ?, 'pending', ?, ?, ?, ?)`, approvalID, intentID, requestedAt, expiresAt, document, nonce); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// ExpireIntent expires the pending approval when its intent expires.
func ExpireIntent(ctx context.Context, tx *sql.Tx, intentID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = 'expired' WHERE intent_id = ? AND status = 'pending'`, intentID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Expire records expiry of one still-pending approval.
func Expire(ctx context.Context, tx *sql.Tx, approvalID, now string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = 'expired', decided_at = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`, now, "approval_expired", approvalID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Resolve records a principal decision on a still-pending approval.
func Resolve(ctx context.Context, tx *sql.Tx, approvalID, status, approver, relay, reason, now string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = ?, decided_at = ?, approver_identity = ?, relay_identity = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`, status, now, approver, relay, reason, approvalID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// BindAssertion binds the verified assertion without changing the single-use nonce.
func BindAssertion(ctx context.Context, tx *sql.Tx, approvalID string, digest []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET assertion_sha256 = ?, nonce = nonce WHERE approval_id = ? AND status = 'pending'`, digest, approvalID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Withdraw invalidates a pending approval bound to a superseded Situation version.
func Withdraw(ctx context.Context, tx *sql.Tx, approvalID, now string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = 'denied', decided_at = ?, withdrawn_at = ?, withdrawal_reason = ?, reason = ? WHERE approval_id = ? AND status = 'pending'`, now, now, "situation_version_conflict", "approval_withdrawn", approvalID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
