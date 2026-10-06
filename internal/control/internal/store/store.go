package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Interlock is the governance interlock reader used for dispatch readiness.
type Interlock = interlock.Reader

// Recovery runs the caller's recovery writes on the claiming transaction.
type Recovery func(*sql.Tx, time.Time) error

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
// the caller's transaction, one opened by WithTx, or autocommit.
type Tx struct {
	q  querier
	tx *sql.Tx
}

// New binds the database without opening a transaction.
func New(db *storage.DB) Store { return Store{db: db} }

// Configured reports whether the database was supplied.
func (s Store) Configured() bool { return s.db != nil }

// Join wraps a transaction the caller owns; a nil transaction yields a Tx that
// reports itself as not open.
func Join(tx *sql.Tx) *Tx {
	if tx == nil {
		return &Tx{}
	}
	return &Tx{q: tx, tx: tx}
}

// Open reports whether the unit of work has a transaction or database behind it.
func (t *Tx) Open() bool { return t.q != nil }

// WithTx opens one original unit of work.
func (s Store) WithTx(ctx context.Context, work func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error { return work(Join(tx)) })
}

// Autocommit runs each statement on its own, outside any transaction.
func (s Store) Autocommit() *Tx { return &Tx{q: s.db} }
