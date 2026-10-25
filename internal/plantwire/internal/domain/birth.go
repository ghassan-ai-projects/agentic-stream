package domain

import "slices"

type Birth struct {
	Envelope
	ProtocolMin      int64   `json:"protocol_min"`
	ProtocolMax      int64   `json:"protocol_max"`
	ContractVersion  string  `json:"contract_version"`
	ContractDigest   string  `json:"contract_digest"`
	Environment      string  `json:"environment"`
	BDSeq            uint64  `json:"bd_seq"`
	PriorLastSeq     int64   `json:"prior_last_seq"`
	CapabilityDigest string  `json:"capability_digest"`
	Kinds            []Kind  `json:"kinds"`
	Boards           []Board `json:"boards"`
}

type Board struct {
	Board       string   `json:"board"`
	Environment string   `json:"environment"`
	Members     []string `json:"members"`
}

func (b Birth) carries(device string) bool {
	return slices.ContainsFunc(b.Boards, func(board Board) bool { return slices.Contains(board.Members, device) })
}
