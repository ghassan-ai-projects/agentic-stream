package domain

import (
	"errors"
	"path/filepath"
	"strings"
)

// Live socket limits.
const (
	DefaultLiveQueueSize = 128
	MaxLiveClients       = 16
	// MaxLineBytes caps per-line memory for file and socket sources at the
	// documented event size.
	MaxLineBytes  = 64 * 1024
	LiveSourceTag = "live-uds"
)

var errSocketPath = errors.New("live socket must be a clean absolute Unix path")

// ErrLineTooLarge means a live line exceeded MaxLineBytes.
var ErrLineTooLarge = errors.New("live ingress line exceeds maximum size")

// LiveLine is one framed line from a socket client. ReadErr is set when the
// line could not be framed; Data then holds the bounded prefix.
type LiveLine struct {
	ConnectionID uint64
	LineNumber   uint64
	Data         []byte
	ReadErr      error
}

// ValidateSocketPath requires a clean absolute Unix socket path.
func ValidateSocketPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.Contains(path, "\x00") || strings.Contains(path, "://") || filepath.Clean(path) != path {
		return errSocketPath
	}
	return nil
}
