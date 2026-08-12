package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
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

func TestEvidenceSocketDialsOnlyUnix(t *testing.T) {
	dir, err := os.MkdirTemp("", "as-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "evidence.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	server := grpc.NewServer()
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := DialEvidenceSocket(context.Background(), path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if conn.Target() == "" {
		t.Fatal("missing connection target")
	}
}
