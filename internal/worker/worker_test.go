package worker_test

import (
	"os"
	"path/filepath"

	"testing"
	"time"

	"google.golang.org/grpc"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestFacadeExposesProtocolRules(t *testing.T) {
	t.Parallel()
	if worker.ProtocolVersion == "" || worker.ContractVersion == "" || worker.EvidenceToolsFeature == "" {
		t.Fatal("protocol identity missing")
	}
	if err := worker.ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if worker.ValidateBudget(nil) == nil || worker.ValidateEvidenceSocketPath("relative") == nil {
		t.Fatal("invalid budget or socket path accepted")
	}
	if err := worker.ValidateEvidenceSocketPath("/tmp/e.sock"); err != nil {
		t.Fatal(err)
	}
}

func TestFacadeListensAndDialsPrivateSockets(t *testing.T) {
	dir, err := os.MkdirTemp("", "ws")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "e.sock")
	listener, err := worker.ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	for name, dial := range map[string]func() (*grpc.ClientConn, error){
		"evidence": func() (*grpc.ClientConn, error) { return worker.DialEvidenceSocket(t.Context(), path) },
		"worker":   func() (*grpc.ClientConn, error) { return worker.DialEpisodeWorkerSocket(t.Context(), path) },
		"tls-off":  func() (*grpc.ClientConn, error) { return worker.DialEpisodeWorkerSocketTLS(t.Context(), path, nil) },
	} {
		conn, err := dial()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_ = conn.Close()
	}
}
