package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

var (
	ErrBoardName              = errors.New("plantwire: a hardware board name is not a plant wire id")
	ErrBoardTwice             = errors.New("plantwire: a hardware board is named twice")
	ErrBoardsOnSimulatedHost  = errors.New("plantwire: hardware boards need a hardware or mixed host")
	ErrHardwareWithoutConsent = errors.New("plantwire: a hardware or mixed host needs the physical effect profile with live actuation and owner authorization")
	ErrNegativeEpoch          = errors.New("plantwire: the authority epoch is negative")
)

var wireID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type Settings struct {
	Environment     string
	HardwareBoards  []string
	PhysicalConsent bool
	Epoch           int64
}

func CheckSettings(s Settings) error {
	if err := CheckEnvironment(s.Environment); err != nil {
		return err
	}
	if s.Epoch < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeEpoch, s.Epoch)
	}
	if err := checkBoardNames(s.HardwareBoards); err != nil {
		return err
	}
	return s.checkConsent()
}

func checkBoardNames(boards []string) error {
	for i, board := range boards {
		if !wireID.MatchString(board) {
			return fmt.Errorf("%w: %q", ErrBoardName, board)
		}
		if slices.Contains(boards[:i], board) {
			return fmt.Errorf("%w: %q", ErrBoardTwice, board)
		}
	}
	return nil
}

func (s Settings) checkConsent() error {
	if s.Environment == EnvironmentSimulated && len(s.HardwareBoards) > 0 {
		return fmt.Errorf("%w: %v", ErrBoardsOnSimulatedHost, s.HardwareBoards)
	}
	if s.Environment != EnvironmentSimulated && !s.PhysicalConsent {
		return fmt.Errorf("%w: the host is %s", ErrHardwareWithoutConsent, s.Environment)
	}
	return nil
}
