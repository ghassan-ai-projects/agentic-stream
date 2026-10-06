package store

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
)

// Publisher publishes the notification of one withdrawal on the caller's
// transaction, so the withdrawal and its notification commit together.
type Publisher func(context.Context, *sql.Tx, domain.Withdrawal) error

// Tx is an opaque unit of work. It never begins or commits a transaction: it
// is the caller's transaction.
type Tx struct {
	tx *sql.Tx
}

// Join wraps a transaction the caller owns; a nil transaction yields a Tx that
// reports itself as not open.
func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

// Open reports whether the unit of work has a transaction behind it.
func (t *Tx) Open() bool { return t.tx != nil }

// Publish hands the transaction to the publisher for one withdrawal.
func (t *Tx) Publish(ctx context.Context, publish Publisher, withdrawal domain.Withdrawal) error {
	return publish(ctx, t.tx, withdrawal)
}
