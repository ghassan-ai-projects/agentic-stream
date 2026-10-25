package domain

import (
	"errors"
	"fmt"
	"slices"
)

const (
	EnvironmentSimulated = "simulated"
	EnvironmentHardware  = "hardware"
	EnvironmentMixed     = "mixed"
)

var ErrUnknownEnvironment = errors.New("plantwire: the environment is none of simulated, hardware, mixed")

type environmentFault struct {
	pointer string
	reason  string
}

func CheckEnvironment(setting string) error {
	if slices.Contains([]string{EnvironmentSimulated, EnvironmentHardware, EnvironmentMixed}, setting) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUnknownEnvironment, setting)
}

func faultOfEnvironment(host string, b Birth) *environmentFault {
	if fault := claimFault(b); fault != nil {
		return fault
	}
	if host == EnvironmentMixed || host == b.Environment {
		return nil
	}
	return &environmentFault{pointer: "/environment", reason: fmt.Sprintf("the host accepts %s nodes only, the node announces %s", host, b.Environment)}
}

func claimFault(b Birth) *environmentFault {
	if b.Environment == EnvironmentMixed {
		return unmixed(b)
	}
	return strayBoard(b)
}

func strayBoard(b Birth) *environmentFault {
	for i, board := range b.Boards {
		if board.Environment != b.Environment {
			return &environmentFault{pointer: fmt.Sprintf("/boards/%d/environment", i),
				reason: fmt.Sprintf("a %s node needs every board %s, board %q is %s", b.Environment, b.Environment, board.Board, board.Environment)}
		}
	}
	return nil
}

func unmixed(b Birth) *environmentFault {
	has := map[string]bool{}
	for _, board := range b.Boards {
		has[board.Environment] = true
	}
	if has[EnvironmentSimulated] && has[EnvironmentHardware] {
		return nil
	}
	missing := EnvironmentHardware
	if !has[EnvironmentSimulated] {
		missing = EnvironmentSimulated
	}
	return &environmentFault{pointer: "/boards", reason: fmt.Sprintf("a mixed node needs a simulated and a hardware board, none of its %d boards is %s", len(b.Boards), missing)}
}
