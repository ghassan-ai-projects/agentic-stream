package domain

import (
	"errors"
	"fmt"
)

// ErrEpochKilled means the epoch was killed and every later decision under it
// is refused (independently of the worker).
var ErrEpochKilled = errors.New("policy epoch is killed")

// ErrEpochUnbound means a decision has no recorded owner epoch.
var ErrEpochUnbound = errors.New("policy epoch is unbound")

// ErrEpochDraining means the epoch is draining and new episodes are refused
// (in-flight episodes finish under their recorded epoch).
var ErrEpochDraining = errors.New("policy epoch is draining")

// Epoch control states.
const (
	EpochDraining = "draining"
	EpochKilled   = "killed"
)

// CheckControllable accepts only the states an operator may record.
func CheckControllable(state string) error {
	if state != EpochDraining && state != EpochKilled {
		return fmt.Errorf("invalid epoch control state %q", state)
	}
	return nil
}

// RefuseDecision refuses a decision under a killed epoch; draining epochs
// still decide.
func RefuseDecision(state string) error {
	if state == EpochKilled {
		return ErrEpochKilled
	}
	return nil
}

// RefuseOrdinary refuses ordinary action work for a draining or killed epoch.
// It is called only for an epoch that has a control record.
func RefuseOrdinary(state string) error {
	switch state {
	case EpochKilled:
		return ErrEpochKilled
	case EpochDraining:
		return ErrEpochDraining
	default:
		return fmt.Errorf("unknown epoch control state %q", state)
	}
}

// RefuseAdmission refuses new episodes while the epoch is draining or killed.
func RefuseAdmission(state string) error {
	if state == EpochKilled || state == EpochDraining {
		return ErrEpochDraining
	}
	return nil
}
