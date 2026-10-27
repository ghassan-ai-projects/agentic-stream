package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func readAll[T any](ctx context.Context, db storage.Querier, what string, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
	rows, err := storage.QueryAll(ctx, db, what, scan, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	return rows, nil
}
