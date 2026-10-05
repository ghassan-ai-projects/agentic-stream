package policy

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// Service is the public facade over policy use cases.
type Service struct{ app *app.Service }

// EvaluateIntent runs ordered gates and atomically records the outcome on tx.
func (s *Service) EvaluateIntent(ctx context.Context, tx *sql.Tx, r EvaluationRequest) (Result, error) {
	return s.app.EvaluateIntent(ctx, store.Join(tx), r)
}

// ResolveApproval records a human decision and re-evaluates before dispatch.
func (s *Service) ResolveApproval(ctx context.Context, tx *sql.Tx, r ApprovalResolution) (Result, error) {
	return s.app.ResolveApproval(ctx, store.Join(tx), r)
}
