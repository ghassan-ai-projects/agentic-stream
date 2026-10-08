package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

func (s Store) EvidenceEvents(ctx context.Context, tenantID string, eventIDs []string) ([]domain.EvidenceEvent, error) {
	ids, err := json.Marshal(eventIDs)
	if err != nil {
		return nil, fmt.Errorf("encode evidence ids: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT position, event_id, event_type, source, entity_id, event_time, ingested_at
		FROM event_log WHERE tenant_id = ? AND event_id IN (SELECT value FROM json_each(?)) ORDER BY position`, tenantID, string(ids))
	if err != nil {
		return nil, fmt.Errorf("read evidence events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events, err := storage.CollectRows(rows, "evidence events", scanEvidenceEvent)
	if err != nil {
		return nil, fmt.Errorf("read evidence events: %w", err)
	}
	return events, nil
}

func scanEvidenceEvent(rows *sql.Rows) (domain.EvidenceEvent, error) {
	var event domain.EvidenceEvent
	if err := rows.Scan(&event.Position, &event.EventID, &event.EventType, &event.Source, &event.EntityID, &event.EventTime, &event.IngestedAt); err != nil {
		return domain.EvidenceEvent{}, fmt.Errorf("scan evidence event: %w", err)
	}
	return event, nil
}
