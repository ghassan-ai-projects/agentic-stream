package domain

import (
	"fmt"
	"time"
)

// RetentionFloor is the minimum notification retention and the deduplication
// horizon for appended events.
const RetentionFloor = 7 * 24 * time.Hour

// Expired reports whether an event time is older than the deduplication
// horizon at now.
func Expired(eventTime, now time.Time) bool {
	return eventTime.Before(now.Add(-RetentionFloor))
}

// CheckRetention refuses a retention shorter than the floor.
func CheckRetention(retention time.Duration) error {
	if retention < RetentionFloor {
		return fmt.Errorf("notification retention cannot be shorter than seven days")
	}
	return nil
}

// RetentionCutoff is the time before which notifications are retired.
func RetentionCutoff(now time.Time, retention time.Duration) time.Time {
	return now.Add(-retention)
}
