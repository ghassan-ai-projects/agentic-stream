// Package approvalledger owns the durable human approval lifecycle: request,
// expiry, resolution, assertion binding and withdrawal. Every operation runs on
// the caller's transaction. It is a thin facade over internal/app; see
// README.md.
package approvalledger

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// Withdrawal is a pending approval withdrawn because a newer Situation version
// replaced the one it was bound to.
type Withdrawal = domain.Withdrawal

// WithdrawalPublisher publishes the notification of one withdrawal on the
// caller's transaction, so the withdrawal and its notification commit together.
type WithdrawalPublisher = store.Publisher

// ErrPublisherRequired means a withdrawal would be silent.
var ErrPublisherRequired = app.ErrPublisherRequired

// Request records the digest-bound human approval request.
func Request(ctx context.Context, tx *sql.Tx, approvalID, intentID, requestedAt, expiresAt string, document []byte, nonce string) error {
	return app.Request(ctx, store.Join(tx), approvalID, intentID, requestedAt, expiresAt, document, nonce)
}

// ExpireIntent expires the pending approval when its intent expires.
func ExpireIntent(ctx context.Context, tx *sql.Tx, intentID string) error {
	return app.ExpireIntent(ctx, store.Join(tx), intentID)
}

// Expire records expiry of one still-pending approval.
func Expire(ctx context.Context, tx *sql.Tx, approvalID, now string) error {
	return app.Expire(ctx, store.Join(tx), approvalID, now)
}

// Resolve records a principal decision on a still-pending approval.
func Resolve(ctx context.Context, tx *sql.Tx, approvalID, status, approver, relay, reason, now string) error {
	return app.Resolve(ctx, store.Join(tx), approvalID, status, approver, relay, reason, now)
}

// BindAssertion binds the verified assertion without changing the single-use nonce.
func BindAssertion(ctx context.Context, tx *sql.Tx, approvalID string, digest []byte) error {
	return app.BindAssertion(ctx, store.Join(tx), approvalID, digest)
}

// Withdraw invalidates a pending approval bound to a superseded Situation version.
func Withdraw(ctx context.Context, tx *sql.Tx, approvalID, now string) error {
	return app.Withdraw(ctx, store.Join(tx), approvalID, now)
}

// WithdrawSuperseded withdraws pending approvals bound to an older Situation
// version and publishes each withdrawal through publish, in order, in the same
// transaction.
func WithdrawSuperseded(ctx context.Context, tx *sql.Tx, situationID string, replacementVersion int, now string, publish WithdrawalPublisher) error {
	return app.WithdrawSuperseded(ctx, store.Join(tx), situationID, replacementVersion, now, publish)
}
