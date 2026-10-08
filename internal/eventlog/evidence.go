package eventlog

import (
	"context"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// EvidenceEvent locates one logged event an explanation cites: its log
// position, identity, source and times.
type EvidenceEvent = domain.EvidenceEvent

// EvidenceEvents locates the tenant's logged events with the given ids, in
// log order. Ids that were never logged are absent from the result.
func (l *EventLog) EvidenceEvents(ctx context.Context, tenantID string, eventIDs []string) ([]EvidenceEvent, error) {
	return l.service.EvidenceEvents(ctx, tenantID, eventIDs)
}
