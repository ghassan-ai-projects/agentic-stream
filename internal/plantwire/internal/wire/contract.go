package wire

import (
	_ "embed"
	"strings"
)

//go:embed plantwire-contract/DIGEST
var pinnedDigest string

func ContractDigest() string {
	return strings.TrimSpace(pinnedDigest)
}
