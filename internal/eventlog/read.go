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
	var stored storedEvent
	if err := rows.Scan(
		&stored.rec.Position, &stored.rec.TenantID, &stored.rec.PartitionID, &stored.rec.EventID,
		&stored.rec.EventType, &stored.rec.SchemaVersion, &stored.rec.Source, &stored.rec.PartitionKey,
		&stored.rec.EntityType, &stored.rec.EntityID, &stored.eventTime, &stored.observedAt,
		&stored.ingestedAt, &stored.correlationID, &stored.causationID, &stored.traceparent,
		&stored.tracestate, &stored.classification, &stored.qualityJSON, &stored.payloadJSON,
	); err != nil {
		return stored.rec, fmt.Errorf("scan record: %w", err)
	}
	return stored.record()
}

// storedEvent is one event_log row as scanned, before its text columns are
// parsed.
type storedEvent struct {
	rec                                                 Record
	eventTime, ingestedAt, classification               string
	observedAt                                          sql.NullString
	correlationID, causationID, traceparent, tracestate sql.NullString
	qualityJSON, payloadJSON                            []byte
}

// record parses the stored times and rebuilds the normalized envelope.
func (s storedEvent) record() (Record, error) {
	rec := s.rec
	var err error
	if rec.EventTime, err = time.Parse(time.RFC3339Nano, s.eventTime); err != nil {
		return rec, fmt.Errorf("parse event_time: %w", err)
	}
	if rec.IngestedAt, err = time.Parse(time.RFC3339Nano, s.ingestedAt); err != nil {
		return rec, fmt.Errorf("parse ingested_at: %w", err)
	}
	if s.observedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, s.observedAt.String)
		if err != nil {
			return rec, fmt.Errorf("parse observed_at: %w", err)
		}
		rec.ObservedAt = &t
	}
	rec.Envelope, err = s.envelope(rec)
	return rec, err
}

func (s storedEvent) envelope(rec Record) (contractsv1.Envelope, error) {
	env := contractsv1.Envelope{
		ID: rec.EventID, Type: rec.EventType, SchemaVersion: rec.SchemaVersion, TenantID: rec.TenantID,
		Source: rec.Source, PartitionKey: rec.PartitionKey,
		Entity:    contractsv1.EntityRef{Type: rec.EntityType, ID: rec.EntityID},
		EventTime: rec.EventTime, ObservedAt: rec.ObservedAt, IngestedAt: rec.IngestedAt,
		CorrelationID: s.correlationID.String, CausationID: s.causationID.String,
		Traceparent: s.traceparent.String, Tracestate: s.tracestate.String,
		Classification: contractsv1.Classification(s.classification),
	}
	if err := json.Unmarshal(s.qualityJSON, &env.Quality); err != nil {
		return env, fmt.Errorf("unmarshal quality: %w", err)
	}
	if err := json.Unmarshal(s.payloadJSON, &env.Data); err != nil {
		return env, fmt.Errorf("unmarshal payload: %w", err)
	}
	return env, nil
}
