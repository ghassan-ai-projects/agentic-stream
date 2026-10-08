package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

func (s *Service) EvidenceEvents(ctx context.Context, tenantID string, eventIDs []string) ([]domain.EvidenceEvent, error) {
	events, err := s.store.EvidenceEvents(ctx, tenantID, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("evidence events of tenant %s: %w", tenantID, err)
	}
	return events, nil
}
