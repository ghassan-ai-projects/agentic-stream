package transport

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceSocketIsPrivateAndCleansUp(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateDir(t), "evidence.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if mode := modeOf(t, path); mode != 0o600 {
		t.Fatalf("socket mode = %o, want 600", mode)
	}
	if mode := modeOf(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("parent mode = %o, want 700", mode)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket still exists: %v", err)
	}
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func TestListenEvidenceSocketRefusesAnExistingPathItDoesNotOwn(t *testing.T) {
	t.Parallel()
	dir := privateDir(t)
	active := filepath.Join(dir, "active.sock")
	listener, err := ListenEvidenceSocket(active)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	stale := filepath.Join(dir, "stale.sock")
	raw, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	raw.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = raw.Close()
	regular := filepath.Join(dir, "regular.sock")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sock")
	if err := os.Symlink(active, link); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, path, want string }{
		{"live socket", active, "already active"},
		{"stale socket", stale, "occupied or stale"},
		{"regular file", regular, "unsafe existing socket"},
		{"symlink to a live socket", link, "unsafe existing socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, err := ListenEvidenceSocket(tt.path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ListenEvidenceSocket(%s) = %v, %v; want error containing %q", tt.path, got, err, tt.want)
			}
		})
	}
}

func TestListenEvidenceSocketRefusesAParentThatIsNotPrivate(t *testing.T) {
	t.Parallel()
	root := privateDir(t)
	open := filepath.Join(root, "open")
	if err := os.Mkdir(open, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o755); err != nil { //nolint:gosec // The test needs a world-readable directory.
		t.Fatal(err)
	}
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinked := filepath.Join(root, "symlinked")
	if err := os.Symlink(real, symlinked); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, parent, want string }{
		{"group and world accessible directory", open, "not private"},
		{"symlink to a private directory", symlinked, "not a private directory"},
		{"regular file", file, "not a private directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(tt.parent, "evidence.sock")
			if got, err := ListenEvidenceSocket(path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ListenEvidenceSocket(%s) = %v, %v; want error containing %q", path, got, err, tt.want)
			}
		})
	}
}

func TestListenEvidenceSocketCreatesAMissingParentPrivately(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(privateDir(t), "created")
	listener, err := ListenEvidenceSocket(filepath.Join(parent, "evidence.sock"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if mode := modeOf(t, parent); mode != 0o700 {
		t.Fatalf("created parent mode = %o, want 700", mode)
	}
}

func TestClosingTheListenerKeepsAReplacedSocket(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateDir(t), "owned.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cleanup removed a file the listener did not own: %v", err)
	}
}

func TestClosingTheListenerToleratesAnAlreadyRemovedSocket(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateDir(t), "gone.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close after removal: %v", err)
	}
}
