package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// EvidenceEvents locates the tenant's logged events with the given ids, in
// log order.
func (s *Service) EvidenceEvents(ctx context.Context, tenantID string, eventIDs []string) ([]domain.EvidenceEvent, error) {
	return s.store.EvidenceEvents(ctx, tenantID, eventIDs) //nolint:wrapcheck // The store names the failed read.
}
