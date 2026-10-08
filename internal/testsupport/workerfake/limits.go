package workerfake

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

const (
	defaultMaxRequestBytes = 4 << 20
	defaultMaxEventBytes   = 1 << 20
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
		MaxRequestBytes: orDefault(l.MaxRequestBytes, defaultMaxRequestBytes),
		MaxEventBytes:   orDefault(l.MaxEventBytes, defaultMaxEventBytes),
		MaxEvents:       orDefault(l.MaxEvents, worker.DefaultMaxEvents),
		MaxStreamBytes:  orDefault(l.MaxStreamBytes, worker.DefaultMaxStreamBytes),
	}
}

// WireError builds a gRPC status error, the protocol's error vocabulary.
func WireError(code codes.Code, message string) error {
	return status.Error(code, message) //nolint:wrapcheck // This is the gRPC wire boundary.
}

// WireErrorf is WireError with a format.
func WireErrorf(code codes.Code, format string, args ...any) error {
	return status.Errorf(code, format, args...) //nolint:wrapcheck // This is the gRPC wire boundary.
}
