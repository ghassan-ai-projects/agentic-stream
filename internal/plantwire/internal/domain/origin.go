package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

const RefusalOriginNotAuthorized = "origin_not_authorized"

var ErrOriginNotAuthorized = errors.New("plantwire: the host may not command a device on this board")

type Origin struct {
	Board       string
	Environment string
}

func (s Settings) Authorizes(origin Origin) error {
	switch {
	case origin.Environment == EnvironmentSimulated:
		return nil
	case origin.Environment == EnvironmentHardware && s.PhysicalConsent && slices.Contains(s.HardwareBoards, origin.Board):
		return nil
	}
	return fmt.Errorf("%w: board %q is %s", ErrOriginNotAuthorized, origin.Board, cmp.Or(origin.Environment, "not announced by the node"))
}

func boardEnvironments(b Birth) map[string]string {
	environments := make(map[string]string, len(b.Boards))
	for _, board := range b.Boards {
		environments[board.Board] = board.Environment
	}
	return environments
}

func (g *ingest) originOf(device *liveDevice) Origin {
	return Origin{Board: device.board, Environment: g.boards[device.board]}
}
