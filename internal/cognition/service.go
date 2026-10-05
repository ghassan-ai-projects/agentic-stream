// Package cognition exposes the deterministic cognitive scheduler facade.
package cognition

import (
	"context"
	"database/sql"

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
