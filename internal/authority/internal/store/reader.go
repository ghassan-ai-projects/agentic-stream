package store

import (
	"context"
	"database/sql"
)

// reader runs a single-row query; *sql.Tx and *storage.DB satisfy it.
type reader interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// nullable stores an empty value as SQL NULL.
func nullable[T comparable](value T) any {
	var zero T
	if value == zero {
		return nil
	}
	return value
}
