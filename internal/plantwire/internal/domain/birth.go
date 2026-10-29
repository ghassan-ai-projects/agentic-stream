package domain

import "slices"

type Birth struct {
	Envelope
	ProtocolMin      int64       `json:"protocol_min"`
	ProtocolMax      int64       `json:"protocol_max"`
	ContractVersion  string      `json:"contract_version"`
	ContractDigest   string      `json:"contract_digest"`
	Environment      string      `json:"environment"`
	BDSeq            uint64      `json:"bd_seq"`
	PriorLastSeq     int64       `json:"prior_last_seq"`
	CapabilityDigest string      `json:"capability_digest"`
	Kinds            []Kind      `json:"kinds"`
	Boards           []Board     `json:"boards"`
	NodeMetrics      NodeMetrics `json:"node_metrics"`
}

const ClockModeWall = "wall"

type NodeMetrics struct {
	ClockMode string `json:"clock_mode"`
}

func (b Birth) PacedWorld() bool {
	return b.Environment == EnvironmentSimulated && b.NodeMetrics.ClockMode != ClockModeWall
}

type Board struct {
	Board       string   `json:"board"`
	Environment string   `json:"environment"`
	Members     []string `json:"members"`
}

func (b Birth) carries(device string) bool {
	return slices.ContainsFunc(b.Boards, func(board Board) bool { return slices.Contains(board.Members, device) })
}

func (b Birth) KindDigests() map[string]string {
	digests := make(map[string]string, len(b.Kinds))
	for _, kind := range b.Kinds {
		digests[kind.Name()] = kind.DeclaredDigest()
	}
	return digests
}
