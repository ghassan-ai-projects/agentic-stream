package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"time"
)

// ReadRequest selects a range of records from the log.
type ReadRequest struct {
	TenantID      string
	PartitionID   int
	AfterPosition LogPosition
	Limit         int
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
