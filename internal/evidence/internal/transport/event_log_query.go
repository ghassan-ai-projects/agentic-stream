package transport

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EventLogQuery answers authorized evidence calls from the normalized event
// log, scoped to the call's tenant, entity, time window and row limit.
func EventLogQuery(db *storage.DB) domain.Query {
	return func(ctx context.Context, call domain.Call) (domain.QueryResult, error) {
		rows, err := readEventRows(ctx, db, call)
		if err != nil {
			return domain.QueryResult{}, err
		}
		result, err := wire.EncodeEvents(rows)
		if err != nil {
			return domain.QueryResult{}, err
		}
		return domain.QueryResult{JSON: result, RowCount: uint64(len(rows))}, nil //nolint:gosec // Result rows are bounded by the authenticated capability.
	}
}

func readEventRows(ctx context.Context, db *storage.DB, call domain.Call) ([]wire.EventRecord, error) {
	rows := make([]wire.EventRecord, 0)
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

func eventRow(event eventlog.EntityEvent) (wire.EventRecord, error) {
	return wire.DecodeEvent(event.EventID, event.EventType, event.EventTime, event.Payload)
}
