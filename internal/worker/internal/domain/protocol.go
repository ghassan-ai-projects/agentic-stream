package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	ProtocolVersion      = "1.0"
	ContractVersion      = "1.0"
	EvidenceToolsFeature = "evidence_tools.v1"

	DefaultMaxEvents      = 4096
	DefaultMaxStreamBytes = 16 << 20
)

func ValidateEvidenceSocketPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.Contains(path, "\x00") || strings.Contains(path, "://") || filepath.Clean(path) != path {
		return fmt.Errorf("evidence socket must be a clean absolute Unix path")
	}
	return nil
}
