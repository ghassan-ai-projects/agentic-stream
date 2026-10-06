package store

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// querier is what both a database handle and a transaction can run.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store keeps the database private. It never exposes a raw transaction.
type Store struct {
	db *storage.DB
}

// Tx is an opaque unit of work. It never begins or commits a transaction: it is
// the caller's transaction, a transaction opened by WithTx, or autocommit.
type Tx struct {
	q querier
}

// New binds the database without opening a transaction.
func New(db *storage.DB) Store { return Store{db: db} }

// Configured reports whether the database was supplied.
func (s Store) Configured() bool { return s.db != nil }

// Join wraps a transaction the caller owns.
func Join(tx *sql.Tx) *Tx { return &Tx{q: tx} }

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, work func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error { return work(Join(tx)) })
}

// Autocommit runs each statement on its own, outside any transaction.
func (s Store) Autocommit() *Tx { return &Tx{q: s.db} }
