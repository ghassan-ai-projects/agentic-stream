// Package store owns evidence SQL and transaction/owner plumbing.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store keeps the database and runtime assertion private.
type Store struct {
	db    *storage.DB
	owner storage.OwnerCheck
	epoch string
}

// Tx is an opaque caller-owned transaction.
type Tx struct {
	tx    *sql.Tx
	owner storage.OwnerCheck
	epoch string
}

// New binds the persistence ports without opening a transaction.
func New(db *storage.DB, owner storage.OwnerCheck, epoch string) Store {
	return Store{db: db, owner: owner, epoch: epoch}
}

// Configured reports whether all persistence safety ports were supplied.
func (s Store) Configured() bool { return s.db != nil && s.owner != nil && s.epoch != "" }

// Join preserves the caller's recovery transaction.
func (s Store) Join(tx *sql.Tx) *Tx { return &Tx{tx: tx, owner: s.owner, epoch: s.epoch} }

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error { return use(s.Join(tx)) })
}

// Configured reports whether a joined transaction is usable.
func (tx *Tx) Configured() bool {
	return tx != nil && tx.tx != nil && tx.owner != nil && tx.epoch != ""
}

// AssertOwner invokes the required assertion in the same transaction.
func (tx *Tx) AssertOwner(ctx context.Context) error {
	if err := tx.owner(ctx, tx.tx, tx.epoch); err != nil {
		return fmt.Errorf("evidence ledger runtime ownership lost: %w", err)
	}
	return nil
}
