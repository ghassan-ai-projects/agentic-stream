package device

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
)

// UDSTransport is a gateway link over a Unix domain socket.
type UDSTransport = transport.UDS

// DialUDSTransport connects to the device gateway listening on the Unix socket
// at path.
func DialUDSTransport(ctx context.Context, path string) (*UDSTransport, error) {
	return transport.Dial(ctx, path) //nolint:wrapcheck // Dial names the socket path.
}
