// Package cognition exposes the deterministic cognitive scheduler facade.
package cognition

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

type Config = app.Config
type Service struct{ app *app.Service }

func New(c Config) (*Service, error) {
	s, err := app.New(c)
	if err != nil {
		return nil, err
	}
	return &Service{app: s}, nil
}
func (s *Service) Process(ctx context.Context, tx *sql.Tx, v situations.Version) error {
	return s.app.Process(ctx, store.Join(tx), v)
}
func RecordCostRejectionReason(ctx context.Context, tx *sql.Tx, itemID string, rejection error) error {
	return app.RecordCostRejectionReason(ctx, store.Join(tx), itemID, rejection)
}

// RecordSchedulerExpiryReason explains on the trigger evaluation why its scheduler item expired.
func RecordSchedulerExpiryReason(ctx context.Context, tx *sql.Tx, itemID, reason string) error {
	return app.RecordSchedulerExpiryReason(ctx, store.Join(tx), itemID, reason)
}

// PruneIgnoredEvaluations removes, inside the caller's transaction, the
// tenant's trigger evaluations older than before whose outcome was ignored and
// that no scheduler item or reconsideration references. Admitted, deferred,
// coalesced, superseded, expired and rejected evaluations are always kept.
func PruneIgnoredEvaluations(ctx context.Context, tx *sql.Tx, tenantID string, before time.Time) (int64, error) {
	return app.PruneIgnoredEvaluations(ctx, store.Join(tx), tenantID, before)
}
