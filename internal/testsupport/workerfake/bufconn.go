package workerfake

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// Connect serves server on an in-memory connection for the test and returns
// the connection a runtime would hold. Both ends are closed when the test
// ends.
func Connect(tb testing.TB, server runtimev1.EpisodeWorkerServer) *grpc.ClientConn {
	tb.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	tb.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		tb.Fatalf("dial in-memory worker: %v", err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	return conn
}

// ConnectClient is Connect for callers that need only the worker client.
func ConnectClient(tb testing.TB, server runtimev1.EpisodeWorkerServer) runtimev1.EpisodeWorkerClient {
	tb.Helper()
	return runtimev1.NewEpisodeWorkerClient(Connect(tb, server))
}
