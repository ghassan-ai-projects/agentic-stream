package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

// Busy-retry and checkpoint tuning.
const (
	RetryAttempts      = 6
	WALCheckpointPages = 256

	retryInitialDelay = 25 * time.Millisecond
	retryMaximumDelay = time.Second
)

// ConnectionString is the SQLite DSN every runtime database opens with: foreign
// keys on, a 30 second busy timeout, WAL with periodic passive checkpoints and
// immediate write transactions.
func ConnectionString(path string) string {
	return fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(%d)&_txlock=immediate", path, WALCheckpointPages)
}

// RetryDelay is the bounded exponential wait before retry attempt+1.
func RetryDelay(attempt int) time.Duration {
	delay := retryInitialDelay << attempt
	if delay > retryMaximumDelay {
		return retryMaximumDelay
	}
	return delay
}

// ShouldRetry reports whether a busy failure on this attempt gets another try.
func ShouldRetry(busy bool, attempt int) bool {
	return busy && attempt < RetryAttempts-1
}

// PendingMigrations keeps the ordered migrations that have not been applied.
func PendingMigrations(all []migrations.Migration, applied map[int]struct{}) []migrations.Migration {
	pending := make([]migrations.Migration, 0, len(all))
	for _, m := range all {
		if _, ok := applied[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	return pending
}

// ReservationSuffix marks a database path reserved by an isolated replay.
const ReservationSuffix = ".replay-reservation"

// ReservationPath is the reservation directory of a database path.
func ReservationPath(path string) string { return path + ReservationSuffix }

// FreshSidecars are the files whose presence means a database path is not fresh.
func FreshSidecars(path string) []string { return []string{path + "-wal", path + "-shm"} }
