package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// Request records the digest-bound human approval request.
func Request(ctx context.Context, tx *store.Tx, approvalID, intentID, requestedAt, expiresAt string, document []byte, nonce string) error {
	return tx.InsertPending(ctx, approvalID, intentID, requestedAt, expiresAt, document, nonce)
}

// ExpireIntent expires the pending approval when its intent expires.
func ExpireIntent(ctx context.Context, tx *store.Tx, intentID string) error {
	return tx.ExpirePendingOfIntent(ctx, intentID)
}

// Expire records expiry of one still-pending approval.
func Expire(ctx context.Context, tx *store.Tx, approvalID, now string) error {
	return tx.ExpirePending(ctx, approvalID, now, domain.ReasonExpired)
}

// Resolve records a principal decision on a still-pending approval.
func Resolve(ctx context.Context, tx *store.Tx, approvalID, status, approver, relay, reason, now string) error {
	return tx.DecidePending(ctx, approvalID, status, approver, relay, reason, now)
}

// BindAssertion binds the verified assertion without changing the single-use nonce.
func BindAssertion(ctx context.Context, tx *store.Tx, approvalID string, digest []byte) error {
	return tx.BindAssertionDigest(ctx, approvalID, digest)
}

// Withdraw invalidates a pending approval bound to a superseded Situation version.
func Withdraw(ctx context.Context, tx *store.Tx, approvalID, now string) error {
	return tx.WithdrawPending(ctx, approvalID, now, domain.WithdrawalConflict, domain.ReasonWithdrawn)
}
