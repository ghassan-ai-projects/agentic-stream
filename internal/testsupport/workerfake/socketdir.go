package workerfake

import (
	"os"
	"testing"
)

// SocketDir returns a private directory for a Unix socket. The worker
// protocol requires a 0700 parent and refuses to listen otherwise, and a
// socket path must stay under the 104-byte macOS limit, which t.TempDir paths
// exceed because they embed the test name.
func SocketDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.MkdirTemp("", "as-sock-") //nolint:usetesting // t.TempDir paths are too long for a Unix socket on macOS and are created with the process umask.
	if err != nil {
		tb.Fatalf("create socket directory: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
