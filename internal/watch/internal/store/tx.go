package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store keeps the database, the runtime ownership check and the interlock
// private. It never exposes a raw transaction.
type Store struct {
	db    *storage.DB
	owner storage.OwnerCheck
	epoch string
}

// Tx is an opaque unit of work. It never begins or commits a transaction.
type Tx struct {
	tx    *sql.Tx
	owner storage.OwnerCheck
	epoch string
}

// New binds the persistence ports without opening a transaction.
func New(db *storage.DB, owner storage.OwnerCheck, epoch string) Store {
	return Store{db: db, owner: owner, epoch: epoch}
}

// Configured reports whether every persistence safety port was supplied.
func (s Store) Configured() bool { return s.db != nil && s.owner != nil }

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		return use(&Tx{tx: tx, owner: s.owner, epoch: s.epoch})
	})
}

func (s Store) RetryBusy(ctx context.Context, fn func() error) error {
	if err := storage.RetrySQLiteBusy(ctx, fn); err != nil {
		return fmt.Errorf("retry watch work on sqlite busy: %w", err)
	}
	return nil
}

// AssertOwner requires the configured runtime owner and epoch in this transaction.
func (tx *Tx) AssertOwner(ctx context.Context) error {
	if err := tx.owner(ctx, tx.tx, tx.epoch); err != nil {
		return fmt.Errorf("assert watch runtime owner: %w", err)
	}
	return nil
}

// AssertInterlock requires the global interlock to be ready in this transaction.
func (tx *Tx) AssertInterlock(ctx context.Context) error {
	if err := interlock.Assert(ctx, tx.tx); err != nil {
		return fmt.Errorf("assert watch interlock: %w", err)
	}
	return nil
}
