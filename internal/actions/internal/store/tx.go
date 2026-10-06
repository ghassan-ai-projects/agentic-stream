package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// OwnerCheck asserts runtime ownership of an epoch inside the transaction.
type OwnerCheck func(context.Context, *sql.Tx, string) error

// Store keeps the database, the runtime ownership check and the interlock
// private. It never exposes a raw transaction.
type Store struct {
	db        *storage.DB
	owner     OwnerCheck
	epoch     string
	interlock interlock.Reader
}

// Tx is an opaque unit of work. It never begins or commits a transaction.
type Tx struct {
	tx        *sql.Tx
	owner     OwnerCheck
	epoch     string
	interlock interlock.Reader
}

// New binds the persistence ports without opening a transaction.
func New(db *storage.DB, owner OwnerCheck, epoch string, reader interlock.Reader) Store {
	return Store{db: db, owner: owner, epoch: epoch, interlock: reader}
}

// Configured reports whether every persistence safety port was supplied.
func (s Store) Configured() bool {
	return s.db != nil && s.owner != nil && s.interlock != nil
}

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		return use(&Tx{tx: tx, owner: s.owner, epoch: s.epoch, interlock: s.interlock})
	})
}

// DispatchAuthorization binds the final readiness gate that a concrete effector
// runs immediately before it accepts one command.
func (s Store) DispatchAuthorization(tenantID, target string) actionport.Authorization {
	return control.NewDispatchAuthorization(s.db, s.interlock, tenantID, target)
}

// JoinCaller wraps a caller-owned transaction for reads that need no ownership
// fence.
func JoinCaller(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

// AssertOwner requires the configured runtime owner and epoch in this transaction.
func (tx *Tx) AssertOwner(ctx context.Context) error {
	if tx.owner == nil {
		return fmt.Errorf("action runtime owner is not configured")
	}
	if err := tx.owner(ctx, tx.tx, tx.epoch); err != nil {
		return fmt.Errorf("action runtime ownership lost: %w", err)
	}
	return nil
}

// AssertInterlock asks the configured final readiness gate in this transaction.
func (tx *Tx) AssertInterlock(ctx context.Context, tenantID, target, risk string) error {
	if tx.interlock == nil {
		return fmt.Errorf("action interlock is not configured")
	}
	if err := tx.interlock.Assert(ctx, tx.tx, tenantID, target, risk); err != nil {
		return fmt.Errorf("interlock rejected command: %w", err)
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
