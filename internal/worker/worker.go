package worker

import (
	"context"
	"crypto/tls"
	"net"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker/internal/transport"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// Protocol identity, the evidence feature and the default size limits.
const (
	ProtocolVersion      = domain.ProtocolVersion
	ContractVersion      = domain.ContractVersion
	EvidenceToolsFeature = domain.EvidenceToolsFeature

	DefaultMaxEvents      = domain.DefaultMaxEvents
	DefaultMaxStreamBytes = domain.DefaultMaxStreamBytes
)

// ValidateBudget requires a worker request to carry a finite wall-time boundary.
func ValidateBudget(budget *runtimev1.EpisodeBudget) error {
	return domain.ValidateBudget(budget) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

// ValidateEvidenceSocketPath accepts only an absolute local filesystem path.
func ValidateEvidenceSocketPath(path string) error {
	return domain.ValidateEvidenceSocketPath(path) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

// ListenEvidenceSocket creates a private runtime-owned Unix socket.
func ListenEvidenceSocket(path string) (net.Listener, error) {
	return transport.ListenEvidenceSocket(path) //nolint:wrapcheck // The transport names the failed step.
}

// DialEpisodeWorkerSocketTLS dials an EpisodeWorker over a Unix socket, using TLS
// when tlsConfig is non-nil.
func DialEpisodeWorkerSocketTLS(ctx context.Context, path string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	return transport.DialEpisodeWorkerSocketTLS(ctx, path, tlsConfig) //nolint:wrapcheck // The transport names the failed step.
}
