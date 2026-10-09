package transport

import (
	"os"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "as-") //nolint:usetesting // t.TempDir paths are too long for a Unix socket on macOS; the socket parent must also be 0700.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
