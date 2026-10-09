package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (u *Unit) InsertEvent(ctx context.Context, tenantID string, env contractsv1.Envelope, body domain.EncodedEvent, createdAt time.Time) (domain.LogPosition, error) {
	res, err := u.tx.ExecContext(ctx, insertEventSQL, eventColumns(tenantID, env, body, createdAt)...)
	if err != nil {
		return -1, fmt.Errorf("insert event: %w", err)
	}
	return insertedPosition(res)
}

const insertEventSQL = `
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
		ON CONFLICT(tenant_id, event_id) DO NOTHING`

func eventColumns(tenantID string, env contractsv1.Envelope, body domain.EncodedEvent, createdAt time.Time) []any {
	return []any{
		tenantID, env.PartitionID(0), env.ID, env.Type, env.SchemaVersion,
		env.Source, env.PartitionKey, env.Entity.Type, env.Entity.ID, kernel.FormatTime(env.EventTime),
		nullableTime(env.ObservedAt), kernel.FormatTime(env.IngestedAt), storage.NullIfEmpty(env.CorrelationID), storage.NullIfEmpty(env.CausationID),
		storage.NullIfEmpty(env.Traceparent), storage.NullIfEmpty(env.Tracestate), string(env.Classification), body.QualityJSON, body.PayloadJSON,
		body.PayloadSHA256, kernel.FormatTime(createdAt),
	}
}

func insertedPosition(res sql.Result) (domain.LogPosition, error) {
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
	return domain.LogPosition(position), nil
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: kernel.FormatTime(*value), Valid: true}
}

func (u *Unit) LoggedEvent(ctx context.Context, tenantID, eventID string) (domain.LoggedEvent, error) {
	var logged domain.LoggedEvent
	if err := u.tx.QueryRowContext(ctx, loggedEventSQL, tenantID, eventID).Scan(
		&logged.EventType, &logged.EntityType, &logged.EntityID, &logged.EventTime, &logged.PayloadSHA256); err != nil {
		return domain.LoggedEvent{}, fmt.Errorf("read logged event %s: %w", eventID, err)
	}
	return logged, nil
}

const loggedEventSQL = `
		SELECT event_type, entity_type, entity_id, event_time, payload_sha256
		FROM event_log WHERE tenant_id = ? AND event_id = ?`
