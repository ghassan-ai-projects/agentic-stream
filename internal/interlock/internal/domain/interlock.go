package domain

import (
	"errors"
	"fmt"
)

const (
	StatusReady   = "ready"
	StatusTripped = "tripped"
)

var ErrTripped = errors.New("runtime interlock is tripped")

func ValidateChange(status, reason string, version int64, now string) error {
	if status != StatusReady && status != StatusTripped {
		return fmt.Errorf("invalid interlock status %q", status)
	}
	if reason == "" || version < 1 || now == "" {
		return fmt.Errorf("interlock reason, version, and timestamp are required")
	}
	return nil
}

type State struct {
	Status, Reason, UpdatedAt string
	Version                   int64
}

func NextState(current State, status, reason, now string) (State, error) {
	next := State{Status: status, Reason: reason, UpdatedAt: now, Version: current.Version + 1}
	if err := ValidateChange(next.Status, next.Reason, next.Version, next.UpdatedAt); err != nil {
		return State{}, err
	}
	return next, nil
}

func RequireReady(status, reason string) error {
	if status != StatusReady {
		return fmt.Errorf("%w: %s", ErrTripped, reason)
	}
	return nil
}
