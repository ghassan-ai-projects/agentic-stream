package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sqliteRetryAttempts      = 6
	sqliteRetryInitialDelay  = 25 * time.Millisecond
	sqliteRetryMaximumDelay  = time.Second
	sqliteWALCheckpointPages = 256
)

// IsSQLiteBusy reports whether err is a retryable SQLite busy or locked
// result, including extended result codes.
func IsSQLiteBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}

// RetrySQLiteBusy retries fn after transient SQLite writer contention. The
// callback must be transaction-safe: a failed transaction must be rolled back
// before it returns so the next attempt starts on a clean connection.
func RetrySQLiteBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < sqliteRetryAttempts; attempt++ {
		err = fn()
		if err == nil || !IsSQLiteBusy(err) || attempt == sqliteRetryAttempts-1 {
			return err
		}

		delay := sqliteRetryInitialDelay << attempt
		if delay > sqliteRetryMaximumDelay {
			delay = sqliteRetryMaximumDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return fmt.Errorf("wait for sqlite retry: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return err
}
