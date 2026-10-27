package domain

import (
	"errors"
	"fmt"
)

const (
	ProtocolVersion int64 = 1
	ContractVersion       = "1.4.0"
	HostID                = "agentic-stream"
	TypeNodeBirth         = "NBIRTH"
	TypeDeviceBirth       = "DBIRTH"
	TypeDeviceData        = "DDATA"
	TypeDeviceDeath       = "DDEATH"
	TypeNodeDeath         = "NDEATH"
	TypeSync              = "NSYNC"
	TypeHostSync          = "HSYNC"
	laneControl           = "control"
)

var ErrOutsideSession = errors.New("plantwire: record from another session or protocol version")

type Envelope struct {
	Type            string `json:"type"`
	ProtocolVersion int64  `json:"protocol_version"`
	SessionID       string `json:"session_id"`
	Seq             uint64 `json:"seq"`
	Lane            string `json:"lane"`
}

func (e Envelope) Within(session string, version int64) error {
	if e.SessionID == session && e.ProtocolVersion == version {
		return nil
	}
	return fmt.Errorf("%w: %s of session %q at version %d", ErrOutsideSession, e.Type, e.SessionID, e.ProtocolVersion)
}
