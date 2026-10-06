package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/domain"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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
	for attempt := 0; attempt < domain.RetryAttempts; attempt++ {
		err = fn()
		if err == nil || !domain.ShouldRetry(IsSQLiteBusy(err), attempt) {
			return err
		}

		if err := waitForSQLiteRetry(ctx, attempt); err != nil {
			return err
		}
	}
	return err
}

// waitForSQLiteRetry bounds backoff and lets cancellation interrupt the wait.
func waitForSQLiteRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(domain.RetryDelay(attempt))
	select {
	case <-ctx.Done():
		timer.Stop()
		return fmt.Errorf("wait for sqlite retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
