package store

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// ReadRecords streams the tenant's scanned records after a position,
// optionally in one partition, in log order with the request's limit.
func (s Store) ReadRecords(ctx context.Context, req domain.ReadRequest, visit func(domain.ScannedEvent) error) error {
	query, args := recordQuery(req)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query event log: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return deliverScannedEvents(rows, visit)
}

// recordQuery selects the tenant's records after a position, optionally in
// one partition, in log order; the limit defaults to 1000.
func recordQuery(req domain.ReadRequest) (string, []any) {
	query := selectRecordsSQL
	args := []any{req.TenantID, req.AfterPosition}
	if req.PartitionID >= 0 {
		query += " AND partition_id = ?"
		args = append(args, req.PartitionID)
	}
	return query + " ORDER BY position LIMIT ?", append(args, domain.ReadLimit(req.Limit))
}

const selectRecordsSQL = `
		SELECT position, tenant_id, partition_id, event_id, event_type,
		       schema_version, source, partition_key, entity_type, entity_id,
		       event_time, observed_at, ingested_at, correlation_id,
		       causation_id, traceparent, tracestate, classification, quality_json,
		       payload_json
		FROM event_log
		WHERE tenant_id = ? AND position > ?`

func deliverScannedEvents(rows *sql.Rows, visit func(domain.ScannedEvent) error) error {
	for rows.Next() {
		scanned, err := scanScannedEvent(rows)
		if err != nil {
			return err
		}
		if err := visit(scanned); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate event log: %w", err)
	}
	return nil
}

func scanScannedEvent(rows *sql.Rows) (domain.ScannedEvent, error) {
	var e domain.ScannedEvent
	var nulls scannedNulls
	if err := rows.Scan(
		&e.Position, &e.TenantID, &e.PartitionID, &e.EventID,
		&e.EventType, &e.SchemaVersion, &e.Source, &e.PartitionKey,
		&e.EntityType, &e.EntityID, &e.EventTime, &nulls.observedAt,
		&e.IngestedAt, &nulls.correlationID, &nulls.causationID, &nulls.traceparent,
		&nulls.tracestate, &e.Classification, &e.QualityJSON, &e.PayloadJSON,
	); err != nil {
		return e, fmt.Errorf("scan record: %w", err)
	}
	e = withScannedNulls(e, nulls)
	return e, nil
}

// scannedNulls holds the row's nullable text columns until projection.
type scannedNulls struct {
	observedAt, correlationID, causationID, traceparent, tracestate sql.NullString
}

func withScannedNulls(e domain.ScannedEvent, nulls scannedNulls) domain.ScannedEvent {
	e.ObservedAt = nullStringPtr(nulls.observedAt)
	e.CorrelationID = nullStringPtr(nulls.correlationID)
	e.CausationID = nullStringPtr(nulls.causationID)
	e.Traceparent = nullStringPtr(nulls.traceparent)
	e.Tracestate = nullStringPtr(nulls.tracestate)
	return e
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// ReadEntityEvents streams one entity window's events in event-time then log
// order, at most MaxRows of them, until visit reports it wants no more.
func (s Store) ReadEntityEvents(ctx context.Context, window domain.EntityWindow, visit func(domain.EntityEvent) (bool, error)) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT event_id, event_type, event_time, payload_json FROM event_log WHERE tenant_id = ? AND entity_id = ? AND event_time >= ? AND event_time <= ? ORDER BY event_time, position LIMIT ?`, window.TenantID, window.EntityID, sources.FormatTime(window.From), sources.FormatTime(window.Until), window.MaxRows)
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

func visitEntityEvents(rows *sql.Rows, visit func(domain.EntityEvent) (bool, error)) error {
	for rows.Next() {
		var event domain.EntityEvent
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
