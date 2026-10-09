package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

const (
	RetryAttempts      = 6
	WALCheckpointPages = 256
	MaxOpenConnections = 8

	retryInitialDelay = 25 * time.Millisecond
	retryMaximumDelay = time.Second
)

func ConnectionString(path string) string {
	return fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(%d)&_txlock=immediate", path, WALCheckpointPages)
}

func RetryDelay(attempt int) time.Duration {
	delay := retryInitialDelay << attempt
	if delay > retryMaximumDelay {
		return retryMaximumDelay
	}
	return delay
}

func ShouldRetry(busy bool, attempt int) bool {
	return busy && attempt < RetryAttempts-1
}

func PendingMigrations(all []migrations.Migration, applied map[int]struct{}) []migrations.Migration {
	pending := make([]migrations.Migration, 0, len(all))
	for _, m := range all {
		if _, ok := applied[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	return pending
}

const ReservationSuffix = ".replay-reservation"

func ReservationPath(path string) string { return path + ReservationSuffix }

func FreshSidecars(path string) []string { return []string{path + "-wal", path + "-shm"} }
