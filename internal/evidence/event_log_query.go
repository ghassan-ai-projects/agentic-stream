package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EventLogQuery answers authorized evidence calls from the normalized event
// log, scoped to the call's tenant, entity, time window and row limit.
func EventLogQuery(db *storage.DB) Query {
	return func(ctx context.Context, call Call) (QueryResult, error) {
		rows, err := readEventRows(ctx, db, call)
		if err != nil {
			return QueryResult{}, err
		}
		result, err := json.Marshal(map[string]any{"rows": rows})
		if err != nil {
			return QueryResult{}, fmt.Errorf("encode evidence result: %w", err)
		}
		return QueryResult{JSON: result, RowCount: uint64(len(rows))}, nil //nolint:gosec // Result rows are bounded by the authenticated capability.
	}
}

func readEventRows(ctx context.Context, db *storage.DB, call Call) ([]map[string]any, error) {
	rows := make([]map[string]any, 0)
	window := eventlog.EntityWindow{TenantID: call.TenantID, EntityID: call.EntityID, From: call.From, Until: call.Until, MaxRows: call.MaxRows}
	err := eventlog.ReadEntityWindow(ctx, db, window, func(event eventlog.EntityEvent) (bool, error) {
		row, err := eventRow(event)
		if err != nil {
			return false, err
		}
		rows = append(rows, row)
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("read evidence events: %w", err)
	}
	return rows, nil
}

func eventRow(event eventlog.EntityEvent) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		return nil, fmt.Errorf("decode evidence payload: %w", err)
	}
	return map[string]any{"event_id": event.EventID, "event_type": event.EventType, "event_time": event.EventTime, "data": data}, nil
}
