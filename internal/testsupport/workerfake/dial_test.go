package workerfake

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

func TestEvidenceSocketDialsOnlyUnix(t *testing.T) {
	dir, err := os.MkdirTemp("", "as-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "evidence.sock")
	listener, err := worker.ListenEvidenceSocket(path)
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
