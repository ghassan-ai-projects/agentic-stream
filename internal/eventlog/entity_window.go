package eventlog

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EntityWindow is a read-only evidence window over one tenant's entity.
type EntityWindow struct {
	TenantID, EntityID string
	From, Until        time.Time
	MaxRows            uint64
}

// EntityEvent is one event of an entity evidence window.
type EntityEvent struct {
	EventID, EventType, EventTime string
	Payload                       []byte
}

// ReadEntityWindow visits the window's events in event-time then log order,
// at most MaxRows of them, until visit reports it wants no more.
func ReadEntityWindow(ctx context.Context, db *storage.DB, window EntityWindow, visit func(EntityEvent) (bool, error)) error {
	rows, err := db.QueryContext(ctx, `SELECT event_id, event_type, event_time, payload_json FROM event_log WHERE tenant_id = ? AND entity_id = ? AND event_time >= ? AND event_time <= ? ORDER BY event_time, position LIMIT ?`, window.TenantID, window.EntityID, window.From.UTC().Format(time.RFC3339Nano), window.Until.UTC().Format(time.RFC3339Nano), window.MaxRows)
	if err != nil {
		return fmt.Errorf("query evidence events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if err := visitEntityEvents(rows, visit); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate evidence events: %w", err)
	}
	return nil
}

func visitEntityEvents(rows *sql.Rows, visit func(EntityEvent) (bool, error)) error {
	for rows.Next() {
		var event EntityEvent
		if err := rows.Scan(&event.EventID, &event.EventType, &event.EventTime, &event.Payload); err != nil {
			return fmt.Errorf("scan evidence event: %w", err)
		}
		more, err := visit(event)
		if err != nil || !more {
			return err
		}
	}
	return nil
}
