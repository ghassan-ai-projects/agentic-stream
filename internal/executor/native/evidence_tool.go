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
	query, err := t.parseQuery(raw, time.Now().UTC())
	if err != nil {
		return ToolResult{}, err
	}
	rows, err := t.readRows(ctx, query)
	if err != nil {
		return ToolResult{}, err
	}
	encoded, err := json.Marshal(map[string]any{"rows": rows})
	if err != nil {
		return ToolResult{}, fmt.Errorf("encode evidence result: %w", err)
	}
	return ToolResult{JSON: encoded, Rows: uint64(len(rows)), Bytes: uint64(len(encoded))}, nil
}

// evidenceQuery is a scoped, bounded evidence read.
type evidenceQuery struct {
	from, until       time.Time
	maxRows, maxBytes uint64
}

// parseQuery applies the caller's arguments, which may only narrow the tool's
// row and byte limits and never widen its entity scope. The window defaults
// to the 24 hours before now.
func (t *SQLiteEvidenceTool) parseQuery(raw json.RawMessage, now time.Time) (evidenceQuery, error) {
	var args struct {
		EntityID string `json:"entity_id"`
		From     string `json:"from"`
		Until    string `json:"until"`
		MaxRows  uint64 `json:"max_rows"`
		MaxBytes uint64 `json:"max_bytes"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return evidenceQuery{}, fmt.Errorf("decode evidence arguments: %w", err)
	}
	if args.EntityID != "" && args.EntityID != t.entityID {
		return evidenceQuery{}, fmt.Errorf("evidence entity is outside episode scope")
	}
	query := evidenceQuery{from: now.Add(-24 * time.Hour), until: now, maxRows: t.maxRows, maxBytes: t.maxBytes}
	if args.MaxRows > 0 && args.MaxRows < query.maxRows {
		query.maxRows = args.MaxRows
	}
	if args.MaxBytes > 0 && args.MaxBytes < query.maxBytes {
		query.maxBytes = args.MaxBytes
	}
	var err error
	if args.From != "" {
		if query.from, err = time.Parse(time.RFC3339Nano, args.From); err != nil {
			return evidenceQuery{}, fmt.Errorf("invalid evidence from: %w", err)
		}
	}
	if args.Until != "" {
		if query.until, err = time.Parse(time.RFC3339Nano, args.Until); err != nil {
			return evidenceQuery{}, fmt.Errorf("invalid evidence until: %w", err)
		}
	}
	if !query.until.After(query.from) {
		return evidenceQuery{}, fmt.Errorf("evidence until must be after from")
	}
	return query, nil
}

// readRows returns the longest prefix of matching events whose encoded
// {"rows":[...]} document stays within maxBytes.
func (t *SQLiteEvidenceTool) readRows(ctx context.Context, query evidenceQuery) ([]json.RawMessage, error) {
	rows, err := t.db.QueryContext(ctx, `SELECT event_id, event_type, event_time, payload_json FROM event_log WHERE tenant_id = ? AND entity_id = ? AND event_time >= ? AND event_time <= ? ORDER BY event_time, position LIMIT ?`, t.tenantID, t.entityID, query.from.UTC().Format(time.RFC3339Nano), query.until.UTC().Format(time.RFC3339Nano), query.maxRows)
	if err != nil {
		return nil, fmt.Errorf("query evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]json.RawMessage, 0)
	// The encoded document is {"rows":[r1,r2,...]}: the envelope, each row,
	// and a comma between rows.
	size := uint64(len(`{"rows":[]}`))
	for rows.Next() {
		var eventID, eventType, eventTime string
		var payload []byte
		if err := rows.Scan(&eventID, &eventType, &eventTime, &payload); err != nil {
			return nil, fmt.Errorf("scan evidence: %w", err)
		}
		var data any
		if err := json.Unmarshal(payload, &data); err != nil {
			return nil, fmt.Errorf("decode evidence payload: %w", err)
		}
		encoded, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": eventType, "event_time": eventTime, "data": data})
		if err != nil {
			return nil, fmt.Errorf("encode evidence: %w", err)
		}
		next := size + uint64(len(encoded))
		if len(result) > 0 {
			next++
		}
		if next > query.maxBytes {
			break
		}
		result = append(result, encoded)
		size = next
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evidence: %w", err)
	}
	return result, nil
}

var _ Tool = (*SQLiteEvidenceTool)(nil)
