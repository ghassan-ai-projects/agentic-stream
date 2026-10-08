package store

import (
	"context"
	"database/sql"
	"fmt"
)

type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func QueryOptional[T any](ctx context.Context, q Querier, query string, args ...any) (value T, found bool, err error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return value, false, fmt.Errorf("run query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return value, false, rowsError(rows)
	}
	if err := rows.Scan(&value); err != nil {
		return value, false, fmt.Errorf("scan optional value: %w", err)
	}
	return value, true, nil
}

func rowsError(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read optional value: %w", err)
	}
	return nil
}
