package store

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store opens the module's units of work and serves its standalone reads.
type Store struct {
	db *storage.DB
}

// New returns a store over db.
func New(db *storage.DB) *Store {
	return &Store{db: db}
}

// Tx is one unit of work. Every load and write of an operation runs on it, so
// the state change and its audit commit or roll back together.
type Tx struct {
	tx *sql.Tx
}

// Join wraps a transaction another module already opened, so reads of this
// module's tables take part in it.
func Join(tx *sql.Tx) *Tx {
	return &Tx{tx: tx}
}

// InTx runs work in one transaction and commits it when work succeeds.
func (s *Store) InTx(ctx context.Context, work func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		return work(&Tx{tx: tx})
	})
}

// Fence is a check owned by another module that must read its own state inside
// this unit of work, such as control.RuntimeOwner.Assert.
type Fence func(ctx context.Context, tx *sql.Tx, epoch string) error

// Assert runs fence for epoch on this unit of work's transaction.
func (t *Tx) Assert(ctx context.Context, fence Fence, epoch string) error {
	return fence(ctx, t.tx, epoch)
}

// OutcomeLedger is another module's transactional read: it counts the
// commands among commandIDs whose outcome is still unresolved, such as
// actions.CountUnresolvedOutcomes.
type OutcomeLedger func(ctx context.Context, tx *sql.Tx, commandIDs []string) (int64, error)

// CountUnresolved runs ledger for commandIDs on this unit of work's
// transaction.
func (t *Tx) CountUnresolved(ctx context.Context, ledger OutcomeLedger, commandIDs []string) (int64, error) {
	return ledger(ctx, t.tx, commandIDs)
}
