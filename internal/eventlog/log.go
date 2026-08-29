// Package eventlog provides the append-only normalized evidence log.
package eventlog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// LogPosition is the durable offset of a record in the event log.
type LogPosition int64

// Record is one stored event-log row.
type Record struct {
	Position      LogPosition
	TenantID      string
	PartitionID   int
	EventID       string
	EventType     string
	SchemaVersion string
	Source        string
	PartitionKey  string
	EntityType    string
	EntityID      string
	EventTime     time.Time
	ObservedAt    *time.Time
	IngestedAt    time.Time
	Envelope      contractsv1.Envelope
}

// ReadRequest selects a range of records from the log.
type ReadRequest struct {
	TenantID      string
	PartitionID   int
	AfterPosition LogPosition
	Limit         int
}

// EventLog appends and reads normalized events.
type EventLog struct {
	db             *storage.DB
	clk            clock.Clock
	requireSchemas bool
}

// RequireSchemaValidation makes ingress validate every envelope against the
// durable event_schemas registry before it can enter event_log.
func (l *EventLog) RequireSchemaValidation() *EventLog {
	l.requireSchemas = true
	return l
}

// ValidateEnvelope checks an envelope against the durable registered schema.
func (l *EventLog) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !l.requireSchemas {
		return nil
	}
	return validateEnvelopeAgainstSchema(ctx, l.db, env)
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func validateEnvelopeAgainstSchema(ctx context.Context, queryer queryRower, env contractsv1.Envelope) error {
	var schemaJSON []byte
	if err := queryer.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", env.Type, env.SchemaVersion).Scan(&schemaJSON); err != nil {
		return fmt.Errorf("event schema %s/%s is not registered: %w", env.Type, env.SchemaVersion, err)
	}
	var schema struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Required             []string `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return fmt.Errorf("decode event schema: %w", err)
	}
	for key, value := range env.Data {
		property, ok := schema.Properties[key]
		if !ok {
			if !schema.AdditionalProperties {
				return fmt.Errorf("payload field %q is not declared by event schema", key)
			}
			continue
		}
		if err := validateJSONSchemaType(property.Type, value); err != nil {
			return fmt.Errorf("payload field %q: %w", key, err)
		}
		if property.Enum != nil {
			if err := validateJSONSchemaEnum(property.Enum, value); err != nil {
				return fmt.Errorf("payload field %q: %w", key, err)
			}
		}
	}
	for _, key := range schema.Required {
		if _, ok := env.Data[key]; !ok {
			return fmt.Errorf("payload field %q is required by event schema", key)
		}
	}
	return nil
}

func validateJSONSchemaEnum(expected []string, value any) error {
	actual, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected one of %v, got %T", expected, value)
	}
	for _, allowed := range expected {
		if actual == allowed {
			return nil
		}
	}
	return fmt.Errorf("expected one of %v, got %q", expected, actual)
}

func validateJSONSchemaType(expected string, value any) error {
	if expected == "" || expected == "null" && value == nil {
		return nil
	}
	switch expected {
	case "number":
		switch value.(type) {
		case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return nil
		}
	case "integer":
		switch number := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return nil
		case float64:
			if number == float64(int64(number)) {
				return nil
			}
		}
	case "string":
		if _, ok := value.(string); ok {
			return nil
		}
	case "boolean":
		if _, ok := value.(bool); ok {
			return nil
		}
	case "object":
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	case "array":
		switch value.(type) {
		case []any, []string, []float64:
			return nil
		}
	default:
		return fmt.Errorf("schema uses unsupported JSON type %q", expected)
	}
	return fmt.Errorf("expected %s, got %T", expected, value)
}

// NewEventLog creates an EventLog backed by db and the physical clock.
func NewEventLog(db *storage.DB) *EventLog {
	return NewEventLogWithClock(db, clock.Physical())
}

// NewEventLogWithClock creates an EventLog backed by db and the given clock.
func NewEventLogWithClock(db *storage.DB, clk clock.Clock) *EventLog {
	if clk == nil {
		clk = clock.Physical()
	}
	return &EventLog{db: db, clk: clk}
}

// Append inserts envelopes into the event log. Duplicate event IDs for the same
// tenant are ignored and reported in the returned positions as -1.
func (l *EventLog) Append(ctx context.Context, tenantID string, envelopes []contractsv1.Envelope) ([]LogPosition, error) {
	positions := make([]LogPosition, len(envelopes))

	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, env := range envelopes {
			if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
				return fmt.Errorf("validate envelope: %w", err)
			}
			if l.requireSchemas {
				if err := validateEnvelopeAgainstSchema(ctx, tx, env); err != nil {
					return err
				}
			}
			pos, err := l.appendOne(ctx, tx, tenantID, env)
			if err != nil {
				return err
			}
			positions[i] = pos
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("append events: %w", err)
	}

	return positions, nil
}

func (l *EventLog) appendOne(ctx context.Context, tx *sql.Tx, tenantID string, env contractsv1.Envelope) (LogPosition, error) {
	if _, err := contractsv1.ParseTraceContext(env.Traceparent, env.Tracestate); err != nil {
		return -1, fmt.Errorf("validate trace context: %w", err)
	}
	payloadJSON, err := json.Marshal(env.Data)
	if err != nil {
		return -1, fmt.Errorf("marshal payload: %w", err)
	}
	payloadHash := sha256.Sum256(payloadJSON)

	qualityJSON, err := json.Marshal(env.Quality)
	if err != nil {
		return -1, fmt.Errorf("marshal quality: %w", err)
	}

	observedAt := sql.NullString{}
	if env.ObservedAt != nil {
		observedAt = sql.NullString{String: env.ObservedAt.Format(time.RFC3339Nano), Valid: true}
	}

	correlationID := sql.NullString{}
	if env.CorrelationID != "" {
		correlationID = sql.NullString{String: env.CorrelationID, Valid: true}
	}
	causationID := sql.NullString{}
	if env.CausationID != "" {
		causationID = sql.NullString{String: env.CausationID, Valid: true}
	}
	traceparent := sql.NullString{}
	if env.Traceparent != "" {
		traceparent = sql.NullString{String: env.Traceparent, Valid: true}
	}
	tracestate := sql.NullString{}
	if env.Tracestate != "" {
		tracestate = sql.NullString{String: env.Tracestate, Valid: true}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO event_log (
			tenant_id, partition_id, event_id, event_type, schema_version,
			source, partition_key, entity_type, entity_id, event_time,
			observed_at, ingested_at, correlation_id, causation_id,
			traceparent, tracestate, classification, quality_json, payload_json,
			payload_sha256, created_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON CONFLICT(tenant_id, event_id) DO NOTHING`,
		tenantID,
		env.PartitionID(0),
		env.ID,
		env.Type,
		env.SchemaVersion,
		env.Source,
		env.PartitionKey,
		env.Entity.Type,
		env.Entity.ID,
		env.EventTime.Format(time.RFC3339Nano),
		observedAt,
		env.IngestedAt.Format(time.RFC3339Nano),
		correlationID,
		causationID,
		traceparent,
		tracestate,
		string(env.Classification),
		qualityJSON,
		payloadJSON,
		payloadHash[:],
		l.clk.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return -1, fmt.Errorf("insert event: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return -1, fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return -1, nil
	}

	position, err := res.LastInsertId()
	if err != nil {
		return -1, fmt.Errorf("last insert id: %w", err)
	}

	return LogPosition(position), nil
}

// Read streams records matching req into callback.
func (l *EventLog) Read(ctx context.Context, req ReadRequest, callback func(Record) error) error {
	if req.Limit <= 0 {
		req.Limit = 1000
	}

	query := `
		SELECT position, tenant_id, partition_id, event_id, event_type,
		       schema_version, source, partition_key, entity_type, entity_id,
		       event_time, observed_at, ingested_at, correlation_id,
		       causation_id, traceparent, tracestate, classification, quality_json,
		       payload_json
		FROM event_log
		WHERE tenant_id = ? AND position > ?`
	args := []any{req.TenantID, req.AfterPosition}
	if req.PartitionID >= 0 {
		query += " AND partition_id = ?"
		args = append(args, req.PartitionID)
	}
	query += " ORDER BY position LIMIT ?"
	args = append(args, req.Limit)
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query event log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return err
		}
		if err := callback(rec); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate event log: %w", err)
	}
	return nil
}

func scanRecord(rows *sql.Rows) (Record, error) {
	var rec Record
	var eventTimeStr, ingestedAtStr string
	var observedAtStr sql.NullString
	var correlationID, causationID, traceparent, tracestate sql.NullString
	var classification string
	var qualityJSON, payloadJSON []byte

	err := rows.Scan(
		&rec.Position,
		&rec.TenantID,
		&rec.PartitionID,
		&rec.EventID,
		&rec.EventType,
		&rec.SchemaVersion,
		&rec.Source,
		&rec.PartitionKey,
		&rec.EntityType,
		&rec.EntityID,
		&eventTimeStr,
		&observedAtStr,
		&ingestedAtStr,
		&correlationID,
		&causationID,
		&traceparent,
		&tracestate,
		&classification,
		&qualityJSON,
		&payloadJSON,
	)
	if err != nil {
		return rec, fmt.Errorf("scan record: %w", err)
	}

	rec.EventTime, err = time.Parse(time.RFC3339Nano, eventTimeStr)
	if err != nil {
		return rec, fmt.Errorf("parse event_time: %w", err)
	}
	rec.IngestedAt, err = time.Parse(time.RFC3339Nano, ingestedAtStr)
	if err != nil {
		return rec, fmt.Errorf("parse ingested_at: %w", err)
	}
	if observedAtStr.Valid {
		t, err := time.Parse(time.RFC3339Nano, observedAtStr.String)
		if err != nil {
			return rec, fmt.Errorf("parse observed_at: %w", err)
		}
		rec.ObservedAt = &t
	}

	rec.Envelope = contractsv1.Envelope{
		ID:             rec.EventID,
		Type:           rec.EventType,
		SchemaVersion:  rec.SchemaVersion,
		TenantID:       rec.TenantID,
		Source:         rec.Source,
		PartitionKey:   rec.PartitionKey,
		Entity:         contractsv1.EntityRef{Type: rec.EntityType, ID: rec.EntityID},
		EventTime:      rec.EventTime,
		ObservedAt:     rec.ObservedAt,
		IngestedAt:     rec.IngestedAt,
		CorrelationID:  correlationID.String,
		CausationID:    causationID.String,
		Traceparent:    traceparent.String,
		Tracestate:     tracestate.String,
		Classification: contractsv1.Classification(classification),
	}
	if err := json.Unmarshal(qualityJSON, &rec.Envelope.Quality); err != nil {
		return rec, fmt.Errorf("unmarshal quality: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &rec.Envelope.Data); err != nil {
		return rec, fmt.Errorf("unmarshal payload: %w", err)
	}

	return rec, nil
}
