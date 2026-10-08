package workerfake

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

// DialEvidenceSocket dials the runtime's evidence socket the way a worker
// does: only a Unix socket with transport credentials that
// carry no remote-network trust. Application capability validation remains
// mandatory for every call.
func DialEvidenceSocket(ctx context.Context, path string) (*grpc.ClientConn, error) {
	if err := worker.ValidateEvidenceSocketPath(path); err != nil {
		return nil, fmt.Errorf("evidence socket: %w", err)
	}
	conn, err := grpc.NewClient("passthrough:///evidence", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		dialCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return (&net.Dialer{}).DialContext(dialCtx, "unix", path)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial evidence socket: %w", err)
	}
	return conn, nil
}
