package transport_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote/internal/transport"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type failingClient struct{ runtimev1.EpisodeWorkerClient }

var errDown = errors.New("down")

func (failingClient) Handshake(context.Context, *runtimev1.HandshakeRequest, ...grpc.CallOption) (*runtimev1.HandshakeResponse, error) {
	return nil, errDown
}

func (failingClient) Execute(context.Context, *runtimev1.EpisodeRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[runtimev1.EpisodeEvent], error) {
	return nil, errDown
}

func TestWorkerWrapsCallFailuresAndReportsConfiguration(t *testing.T) {
	t.Parallel()
	if transport.NewWorker(nil).Configured() {
		t.Fatal("nil client configured")
	}
	worker := transport.NewWorker(failingClient{})
	if !worker.Configured() {
		t.Fatal("client not configured")
	}
	if _, err := worker.Handshake(t.Context(), &runtimev1.HandshakeRequest{}); !errors.Is(err, errDown) {
		t.Fatalf("handshake = %v", err)
	}
	if _, err := worker.Open(t.Context(), &runtimev1.EpisodeRequest{}); !errors.Is(err, errDown) {
		t.Fatalf("open = %v", err)
	}
}
