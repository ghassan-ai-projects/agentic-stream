package store

import (
	"context"
	"database/sql"
	"time"
)

// querier is what both a database handle and a transaction can run.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx is an opaque unit of work. It never begins or commits a transaction: it
// is the caller's transaction, or a read-only database handle.
type Tx struct {
	q  querier
	tx *sql.Tx
}

// Join wraps a transaction the caller owns; a nil transaction yields a Tx that
// reports itself as not open.
func Join(tx *sql.Tx) *Tx {
	if tx == nil {
		return &Tx{}
	}
	return &Tx{q: tx, tx: tx}
}

// Reader wraps a database handle for reads outside a transaction.
func Reader(db *sql.DB) *Tx {
	if db == nil {
		return &Tx{}
	}
	return &Tx{q: db}
}

// Open reports whether the unit of work has a transaction or database behind it.
func (t *Tx) Open() bool { return t.q != nil }

// TimeText encodes a time as UTC RFC 3339 with nanoseconds.
func TimeText(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

// AcceptedAtText encodes accepted_at, which is ordered as SQLite TEXT:
// fixed-width nanoseconds keep lexical order identical to chronological order.
func AcceptedAtText(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
