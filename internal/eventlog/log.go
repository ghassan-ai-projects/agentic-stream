// Package eventlog provides the append-only normalized evidence log.
package eventlog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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
	TenantID     string
	PartitionID  int
	AfterPosition LogPosition
	Limit        int
}

// EventLog appends and reads normalized events.
type EventLog struct {
	db *storage.DB
}

// NewEventLog creates an EventLog backed by db.
func NewEventLog(db *storage.DB) *EventLog {
	return &EventLog{db: db}
}

// Append inserts envelopes into the event log. Duplicate event IDs for the same
// tenant are ignored and reported in the returned positions as -1.
func (l *EventLog) Append(ctx context.Context, tenantID string, envelopes []contractsv1.Envelope) ([]LogPosition, error) {
	positions := make([]LogPosition, len(envelopes))

	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, env := range envelopes {
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

	res, err := tx.ExecContext(ctx, `
		INSERT INTO event_log (
			tenant_id, partition_id, event_id, event_type, schema_version,
			source, partition_key, entity_type, entity_id, event_time,
			observed_at, ingested_at, correlation_id, causation_id,
			traceparent, classification, quality_json, payload_json,
			payload_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		string(env.Classification),
		qualityJSON,
		payloadJSON,
		payloadHash[:],
		time.Now().UTC().Format(time.RFC3339Nano),
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

	rows, err := l.db.QueryContext(ctx, `
		SELECT position, tenant_id, partition_id, event_id, event_type,
		       schema_version, source, partition_key, entity_type, entity_id,
		       event_time, observed_at, ingested_at, correlation_id,
		       causation_id, traceparent, classification, quality_json,
		       payload_json
		FROM event_log
		WHERE tenant_id = ? AND partition_id = ? AND position > ?
		ORDER BY position
		LIMIT ?`,
		req.TenantID, req.PartitionID, req.AfterPosition, req.Limit)
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
	var correlationID, causationID, traceparent sql.NullString
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
