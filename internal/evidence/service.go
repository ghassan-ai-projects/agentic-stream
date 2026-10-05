package evidence

import (
	"context"
	"database/sql"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"time"
)

// Service is the sole public evidence implementation; its operations delegate.
type Service struct {
	runtimev1.UnimplementedEvidenceToolsServer
	app       *app.Service
	transport *transport.Server
}

// Issue signs an attempt-scoped capability.
func (s *Service) Issue(scope Scope) ([]byte, error) { return s.app.Issue(scope) }

// Verify returns authenticated capability claims.
func (s *Service) Verify(token []byte) (Scope, error) { return s.app.Verify(token) }

// IssueTime supplies the configured clock to the remote attempt issuer.
func (s *Service) IssueTime() time.Time { return s.app.IssueTime() }

// Call serves one authorized, bounded, durable worker request.
func (s *Service) Call(ctx context.Context, req *runtimev1.EvidenceToolCall) (*runtimev1.EvidenceToolResult, error) {
	return s.transport.Call(ctx, req)
}

// RuntimeEpoch reports the configured durable owner binding.
func (s *Service) RuntimeEpoch() string { return s.app.RuntimeEpoch() }

// ReclaimExpired interrupts abandoned calls under the runtime owner check.
func (s *Service) ReclaimExpired(ctx context.Context, now time.Time) error {
	return s.app.ReclaimExpired(ctx, now)
}

// RecoverTx joins atomic startup recovery without opening a transaction.
func (s *Service) RecoverTx(ctx context.Context, tx *sql.Tx, now time.Time) (int, error) {
	return s.app.RecoverTx(ctx, s.app.JoinRecovery().Join(tx), now)
}
