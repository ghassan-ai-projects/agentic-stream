// Package duration parses the runtime's duration string format.
package duration

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse converts a duration string such as "15m", "6h", or "30d" into a
// time.Duration. It accepts the same base units as time.ParseDuration plus
// days ("d").
func Parse(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("parse days in %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return d, nil
}
