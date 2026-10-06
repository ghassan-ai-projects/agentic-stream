package domain

import (
	"errors"
	"fmt"
)

// Interlock statuses.
const (
	StatusReady   = "ready"
	StatusTripped = "tripped"
)

// ErrTripped means the action plane is globally blocked by a durable interlock.
var ErrTripped = errors.New("runtime interlock is tripped")

// ValidateChange requires a known status, a reason, a positive version and a
// timestamp.
func ValidateChange(status, reason string, version int64, now string) error {
	if status != StatusReady && status != StatusTripped {
		return fmt.Errorf("invalid interlock status %q", status)
	}
	if reason == "" || version < 1 || now == "" {
		return fmt.Errorf("interlock reason, version, and timestamp are required")
	}
	return nil
}

// RequireReady fails closed unless the stored status is ready, carrying the
// stored reason in the error.
func RequireReady(status, reason string) error {
	if status != StatusReady {
		return fmt.Errorf("%w: %s", ErrTripped, reason)
	}
	return nil
}
