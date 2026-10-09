package transport

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "as-") //nolint:usetesting // A Unix socket path is limited to about 100 bytes; t.TempDir() paths are longer.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "live.sock")
}

func TestListenerIsOwnerOnlyAndRemovesItsSocketOnClose(t *testing.T) {
	t.Parallel()
	path := socketPath(t)
	listener, err := listenSocket(path)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, err = %v, want owner-only", info.Mode().Perm(), err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the socket file must be removed on close: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("a second close: %v", err)
	}
}

func TestListenerRefusesUnsafeExistingPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "file")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"regular file": regular, "symlink": link} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := listenSocket(path); err == nil || !strings.Contains(err.Error(), "refusing unsafe existing live socket path") {
				t.Fatalf("err = %v, want an unsafe-path refusal", err)
			}
		})
	}
}

func TestListenerRefusesAnActiveSocket(t *testing.T) {
	t.Parallel()
	path := socketPath(t)
	first, err := listenSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	if _, err := listenSocket(path); err == nil || !strings.Contains(err.Error(), "live socket is already active") {
		t.Fatalf("err = %v, want an already-active refusal", err)
	}
}

func TestListenerRefusesAStaleSocketFile(t *testing.T) {
	t.Parallel()
	path := socketPath(t)
	stale, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := listenSocket(path); err == nil || !strings.Contains(err.Error(), "occupied or stale") {
		t.Fatalf("err = %v, want a stale-socket refusal", err)
	}
}
