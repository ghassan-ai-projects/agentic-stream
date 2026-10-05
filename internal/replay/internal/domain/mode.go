package domain

import (
	"errors"
	"fmt"
)

// Mode is an effect-safe replay mode. Replay has no credential or resolver
// input by construction; recorded mode uses durable ledgers, shadow reports
// differences without effects, and counterfactual is simulator-only.
type Mode string

const (
	ModeDeterministic  Mode = "deterministic"
	ModeRecorded       Mode = "recorded"
	ModeShadow         Mode = "shadow"
	ModeCounterfactual Mode = "counterfactual"
)

// ErrModeCapabilityRequired means a worker-aware replay mode was requested
// without its explicit ledger, worker, or simulator capability.
var ErrModeCapabilityRequired = errors.New("replay mode capability required")

// ErrUnsupportedMode means the caller supplied a mode outside the frozen
// replay contract.
var ErrUnsupportedMode = errors.New("unsupported replay mode")

// WorkerAwareMode reports whether a mode executes worker-aware capability
// phases after the deterministic stream replay.
func WorkerAwareMode(mode Mode) bool {
	return mode == ModeRecorded || mode == ModeShadow || mode == ModeCounterfactual
}

// AdmitCapabilities accepts at most one explicit capability set per replay.
func AdmitCapabilities(sets []Capabilities) (Capabilities, error) {
	if len(sets) > 1 {
		return Capabilities{}, fmt.Errorf("at most one replay capability set is allowed")
	}
	if len(sets) == 1 {
		return sets[0], nil
	}
	return Capabilities{}, nil
}

// Validate fails closed when a worker-aware mode lacks its explicit capability.
func (c Capabilities) Validate(mode Mode) error {
	switch mode {
	case ModeRecorded:
		return requireReplayCapability(c.RecordedLedger != nil, "recorded ledger")
	case ModeShadow:
		return c.requireShadowExecutors()
	case ModeCounterfactual:
		return requireReplayCapability(c.Simulator != nil, "counterfactual simulator")
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
