package native

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SQLiteEvidenceTool exposes bounded, read-only event evidence to the native
// executor. The tenant and entity are bound by the trusted episode request;
// caller-supplied values cannot widen that scope.
type SQLiteEvidenceTool struct {
	db       *storage.DB
	name     string
	tenantID string
	entityID string
	maxRows  uint64
	maxBytes uint64
}

// NewSQLiteEvidenceTool creates a scoped native evidence tool.
func NewSQLiteEvidenceTool(db *storage.DB, name, tenantID, entityID string) *SQLiteEvidenceTool {
	return &SQLiteEvidenceTool{db: db, name: name, tenantID: tenantID, entityID: entityID, maxRows: 1000, maxBytes: 1 << 20}
}

// Name returns the configured tool name.
func (t *SQLiteEvidenceTool) Name() string { return t.name }

// Call reads bounded event evidence. Supported arguments are entity_id,
// from, until, max_rows, and max_bytes.
func (t *SQLiteEvidenceTool) Call(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	if t == nil || t.db == nil || t.tenantID == "" || t.entityID == "" {
		return ToolResult{}, fmt.Errorf("evidence tool is not configured")
	}
	var args struct {
		EntityID string `json:"entity_id"`
		From     string `json:"from"`
		Until    string `json:"until"`
		MaxRows  uint64 `json:"max_rows"`
		MaxBytes uint64 `json:"max_bytes"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return ToolResult{}, fmt.Errorf("decode evidence arguments: %w", err)
	}
	if args.EntityID != "" && args.EntityID != t.entityID {
		return ToolResult{}, fmt.Errorf("evidence entity is outside episode scope")
	}
	maxRows, maxBytes := t.maxRows, t.maxBytes
	if args.MaxRows > 0 && args.MaxRows < maxRows {
		maxRows = args.MaxRows
	}
	if args.MaxBytes > 0 && args.MaxBytes < maxBytes {
		maxBytes = args.MaxBytes
	}
	from, until := time.Now().UTC().Add(-24*time.Hour), time.Now().UTC()
	var err error
	if args.From != "" {
		from, err = time.Parse(time.RFC3339Nano, args.From)
		if err != nil {
			return ToolResult{}, fmt.Errorf("invalid evidence from: %w", err)
		}
	}
	if args.Until != "" {
		until, err = time.Parse(time.RFC3339Nano, args.Until)
		if err != nil {
			return ToolResult{}, fmt.Errorf("invalid evidence until: %w", err)
		}
	}
	if !until.After(from) {
		return ToolResult{}, fmt.Errorf("evidence until must be after from")
	}
	rows, err := t.db.QueryContext(ctx, `SELECT event_id, event_type, event_time, payload_json FROM event_log WHERE tenant_id = ? AND entity_id = ? AND event_time >= ? AND event_time <= ? ORDER BY event_time, position LIMIT ?`, t.tenantID, t.entityID, from.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano), maxRows)
	if err != nil {
		return ToolResult{}, fmt.Errorf("query evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		var eventID, eventType, eventTime string
		var payload []byte
		if err := rows.Scan(&eventID, &eventType, &eventTime, &payload); err != nil {
			return ToolResult{}, fmt.Errorf("scan evidence: %w", err)
		}
		var data any
		if err := json.Unmarshal(payload, &data); err != nil {
			return ToolResult{}, fmt.Errorf("decode evidence payload: %w", err)
		}
		candidate := append(append([]map[string]any(nil), resultRows...), map[string]any{"event_id": eventID, "event_type": eventType, "event_time": eventTime, "data": data})
		encoded, err := json.Marshal(map[string]any{"rows": candidate})
		if err != nil {
			return ToolResult{}, fmt.Errorf("encode evidence: %w", err)
		}
		if uint64(len(encoded)) > maxBytes {
			break
		}
		resultRows = candidate
	}
	if err := rows.Err(); err != nil {
		return ToolResult{}, fmt.Errorf("iterate evidence: %w", err)
	}
	encoded, err := json.Marshal(map[string]any{"rows": resultRows})
	if err != nil {
		return ToolResult{}, fmt.Errorf("encode evidence result: %w", err)
	}
	return ToolResult{JSON: encoded, Rows: uint64(len(resultRows)), Bytes: uint64(len(encoded))}, nil
}

var _ Tool = (*SQLiteEvidenceTool)(nil)
