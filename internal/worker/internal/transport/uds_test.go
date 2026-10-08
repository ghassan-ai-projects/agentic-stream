package transport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceSocketIsPrivateAndCleansUp(t *testing.T) {
	dir, err := os.MkdirTemp("", "as-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "evidence.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %o", info.Mode().Perm())
	}
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if parentInfo.Mode().Perm() != 0o700 {
		t.Fatalf("parent mode = %o", parentInfo.Mode().Perm())
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket still exists: %v", err)
	}
}
