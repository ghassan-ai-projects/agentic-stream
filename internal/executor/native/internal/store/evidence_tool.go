package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type SQLiteEvidenceTool struct {
	db       *storage.DB
	name     string
	tenantID string
	entityID string
	maxRows  uint64
	maxBytes uint64
	horizon  time.Time
}

func NewSQLiteEvidenceTool(db *storage.DB, name, tenantID, entityID string, horizon time.Time) *SQLiteEvidenceTool {
	return &SQLiteEvidenceTool{db: db, name: name, tenantID: tenantID, entityID: entityID, maxRows: evidence.DefaultReadMaxRows, maxBytes: evidence.DefaultReadMaxBytes, horizon: horizon}
}

func (t *SQLiteEvidenceTool) Name() string { return t.name }

func (t *SQLiteEvidenceTool) Call(ctx context.Context, raw json.RawMessage) (domain.ToolResult, error) {
	if t == nil || t.db == nil || t.tenantID == "" || t.entityID == "" || t.horizon.IsZero() {
		return domain.ToolResult{}, fmt.Errorf("evidence tool is not configured")
	}
	query, err := domain.EvidenceScope{EntityID: t.entityID, MaxRows: t.maxRows, MaxBytes: t.maxBytes, Window: evidence.DefaultReadWindow}.Query(raw, t.horizon)
	if err != nil {
		return domain.ToolResult{}, err
	}
	rows, err := t.readRows(ctx, query)
	if err != nil {
		return domain.ToolResult{}, err
	}
	return encodeEvidenceResult(rows)
}

func (t *SQLiteEvidenceTool) readRows(ctx context.Context, query domain.EvidenceQuery) ([]json.RawMessage, error) {
	rows := boundedRows{maxBytes: query.MaxBytes, size: uint64(len(`{"rows":[]}`)), result: make([]json.RawMessage, 0)}
	window := eventlog.EntityWindow{TenantID: t.tenantID, EntityID: t.entityID, From: query.From, Until: query.Until, MaxRows: query.MaxRows}
	if err := eventlog.ReadEntityWindow(ctx, t.db, window, rows.add); err != nil {
		return nil, fmt.Errorf("query evidence: %w", err)
	}
	return rows.result, nil
}

type boundedRows struct {
	maxBytes, size uint64
	result         []json.RawMessage
}

func (b *boundedRows) add(event eventlog.EntityEvent) (bool, error) {
	encoded, err := encodeEvidenceRow(event)
	if err != nil {
		return false, err
	}
	next := b.size + uint64(len(encoded))
	if len(b.result) > 0 {
		next++
	}
	if next > b.maxBytes {
		return false, nil
	}
	b.result = append(b.result, encoded)
	b.size = next
	return true, nil
}

func encodeEvidenceRow(event eventlog.EntityEvent) (json.RawMessage, error) {
	var data any
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		return nil, fmt.Errorf("decode evidence payload: %w", err)
	}
	encoded, err := json.Marshal(map[string]any{"event_id": event.EventID, "event_type": event.EventType, "event_time": event.EventTime, "data": data})
	if err != nil {
		return nil, fmt.Errorf("encode evidence: %w", err)
	}
	return encoded, nil
}

var _ domain.Tool = (*SQLiteEvidenceTool)(nil)

func encodeEvidenceResult(rows []json.RawMessage) (domain.ToolResult, error) {
	encoded, err := json.Marshal(map[string]any{"rows": rows})
	if err != nil {
		return domain.ToolResult{}, fmt.Errorf("encode evidence result: %w", err)
	}
	return domain.ToolResult{JSON: encoded, Rows: uint64(len(rows)), Bytes: uint64(len(encoded))}, nil
}
