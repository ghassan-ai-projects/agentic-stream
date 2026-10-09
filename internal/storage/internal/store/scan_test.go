package store_test

import "database/sql"

func scanOne[T any](rows *sql.Rows) (T, error) {
	var value T
	err := rows.Scan(&value)
	return value, err
}
