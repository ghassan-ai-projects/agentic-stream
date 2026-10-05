package episodes

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// Service is the public interface to episode assembly and bounded execution.
type Service struct{ app *app.Service }

// Assemble builds a request from a pending scheduler item on the caller's transaction.
func (s *Service) Assemble(ctx context.Context, tx *sql.Tx, itemID, tenantID string) (*Request, error) {
	return s.app.Assemble(ctx, store.Join(tx), itemID, tenantID)
}

// Persist admits the request and scheduler item atomically on the caller's transaction.
func (s *Service) Persist(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	return s.app.Persist(ctx, store.Join(tx), req, now)
}

// RunOnce claims and executes one episode, or refuses an assembly-only service.
func (s *Service) RunOnce(ctx context.Context, tenantID string) (bool, error) {
	return s.app.RunOnce(ctx, tenantID)
}
