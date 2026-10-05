package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// reader runs a single-row query; *sql.Tx and *storage.DB satisfy it.
type reader interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// storedTimeLayout is the fixed-width UTC layout of every stored timestamp, so
// stored times compare correctly as text.
const storedTimeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(value time.Time) string {
	return value.UTC().Format(storedTimeLayout)
}

func parseTime(what, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", what, err)
	}
	return parsed, nil
}

// nullable stores an empty value as SQL NULL.
func nullable[T comparable](value T) any {
	var zero T
	if value == zero {
		return nil
	}
	return value
}
