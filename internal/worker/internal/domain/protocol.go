package domain

import (
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Protocol identity, the one optional feature, and the default size limits.
const (
	ProtocolVersion      = "1.0"
	ContractVersion      = "1.0"
	EvidenceToolsFeature = "evidence_tools.v1"

	DefaultMaxRequestBytes = 4 << 20
	DefaultMaxEventBytes   = 1 << 20
	DefaultMaxEvents       = 4096
	DefaultMaxStreamBytes  = 16 << 20
)

// Limits are the size bounds of one worker connection; a zero field means the
// default.
type Limits struct {
	MaxRequestBytes uint64
	MaxEventBytes   uint64
	MaxEvents       uint64
	MaxStreamBytes  uint64
}

// Resolved replaces every zero bound with its default.
func (l Limits) Resolved() Limits {
	orDefault := func(value, fallback uint64) uint64 {
		if value == 0 {
			return fallback
		}
		return value
	}
	return Limits{
		MaxRequestBytes: orDefault(l.MaxRequestBytes, DefaultMaxRequestBytes),
		MaxEventBytes:   orDefault(l.MaxEventBytes, DefaultMaxEventBytes),
		MaxEvents:       orDefault(l.MaxEvents, DefaultMaxEvents),
		MaxStreamBytes:  orDefault(l.MaxStreamBytes, DefaultMaxStreamBytes),
	}
}

// WireError builds a gRPC status error. The status codes are the protocol's
// error vocabulary, so the wire boundary keeps them unwrapped.
func WireError(code codes.Code, message string) error {
	return status.Error(code, message) //nolint:wrapcheck // This is the gRPC wire boundary.
}

// WireErrorf is WireError with a format.
func WireErrorf(code codes.Code, format string, args ...any) error {
	return status.Errorf(code, format, args...) //nolint:wrapcheck // This is the gRPC wire boundary.
}

// ValidateEvidenceSocketPath is the v1 transport rule: only an absolute local
// filesystem path is accepted. URI schemes and remote endpoints are absent.
func ValidateEvidenceSocketPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.Contains(path, "\x00") || strings.Contains(path, "://") || filepath.Clean(path) != path {
		return fmt.Errorf("evidence socket must be a clean absolute Unix path")
	}
	return nil
}
