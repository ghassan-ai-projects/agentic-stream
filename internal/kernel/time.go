package kernel

import (
	"fmt"
	"time"
)

const timeLayout = "2006-01-02T15:04:05.000000000Z"

// FormatTime is the durable text of an instant: UTC with a fixed nine-digit
// fraction, so text order and SQL comparison are chronological order.
func FormatTime(at time.Time) string {
	return at.UTC().Format(timeLayout)
}

// ParseTime reads durable timestamp text, with or without a fraction, into UTC.
func ParseTime(text string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time: %w", err)
	}
	return parsed.UTC(), nil
}
