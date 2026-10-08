package store

import (
	"database/sql"
	"fmt"
)

// CollectRows scans every remaining row with scan, in order. The caller owns
// and closes rows. A scan error is returned as is; an iteration failure is
// reported as "iterate <what>: <cause>". No rows yields an empty, non-nil slice.
func CollectRows[T any](rows *sql.Rows, what string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", what, err)
	}
	return items, nil
}
