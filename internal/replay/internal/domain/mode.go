package domain

import (
	"errors"
	"fmt"
)

type Mode string

const (
	ModeDeterministic Mode = "deterministic"
	ModeRecorded      Mode = "recorded"
	ModeShadow        Mode = "shadow"
)

var ErrModeCapabilityRequired = errors.New("replay mode capability required")

var ErrUnsupportedMode = errors.New("unsupported replay mode")

func WorkerAwareMode(mode Mode) bool {
	return mode == ModeRecorded || mode == ModeShadow
}

func AdmitCapabilities(sets []Capabilities) (Capabilities, error) {
	if len(sets) > 1 {
		return Capabilities{}, fmt.Errorf("at most one replay capability set is allowed")
	}
	if len(sets) == 1 {
		return sets[0], nil
	}
	return Capabilities{}, nil
}

func (c Capabilities) Validate(mode Mode) error {
	switch mode {
	case ModeRecorded:
		return requireReplayCapability(c.RecordedLedger != nil, "recorded ledger")
	case ModeShadow:
		return c.requireShadowExecutors()
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
}

func (c Capabilities) requireShadowExecutors() error {
	if err := requireReplayCapability(c.BaselineExecutor != nil, "deterministic baseline executor"); err != nil {
		return err
	}
	return requireReplayCapability(c.ShadowExecutor != nil, "shadow executor")
}

func requireReplayCapability(present bool, name string) error {
	if !present {
		return fmt.Errorf("%w: %s", ErrModeCapabilityRequired, name)
	}
	return nil
}
