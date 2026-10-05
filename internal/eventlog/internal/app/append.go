package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
)

// Append inserts envelopes into the event log in one unit of work. Duplicate
// event ids for the same tenant are ignored and reported as -1 positions; a
// later admission failure rolls back earlier records.
func (s *Service) Append(ctx context.Context, tenantID string, envelopes []contractsv1.Envelope) ([]domain.LogPosition, error) {
	positions := make([]domain.LogPosition, len(envelopes))
	if err := s.store.Unit(ctx, func(u *store.Unit) error {
		return s.appendBatch(ctx, u, tenantID, envelopes, positions)
	}); err != nil {
		return nil, fmt.Errorf("append events: %w", err)
	}
	return positions, nil
}

func (s *Service) appendBatch(ctx context.Context, u *store.Unit, tenantID string, envelopes []contractsv1.Envelope, positions []domain.LogPosition) error {
	for i, env := range envelopes {
		if err := s.admit(ctx, u, tenantID, env); err != nil {
			return err
		}
		pos, err := s.appendOne(ctx, u, tenantID, env)
		if err != nil {
			return err
		}
		positions[i] = pos
	}
	return nil
}
