package workerfake

import (
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

func TestDialEvidenceSocketReachesAUnixListener(t *testing.T) {
	t.Parallel()
	path := filepath.Join(SocketDir(t), "evidence.sock")
	listener, err := worker.ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := grpc.NewServer()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := DialEvidenceSocket(t.Context(), path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.Connect()
	for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
		if !conn.WaitForStateChange(t.Context(), state) {
			t.Fatalf("connection stopped in state %s before becoming ready", state)
		}
	}
}

func TestDialEvidenceSocketRefusesAnythingButACleanAbsolutePath(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"", "evidence.sock", "unix:///tmp/evidence.sock", "/tmp/../evidence.sock"} {
		if conn, err := DialEvidenceSocket(t.Context(), path); err == nil {
			_ = conn.Close()
			t.Errorf("DialEvidenceSocket(%q) succeeded", path)
		}
	}
}
