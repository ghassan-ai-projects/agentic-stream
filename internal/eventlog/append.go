package eventlog

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// Append inserts envelopes into the event log. Duplicate event IDs for the same
// tenant are ignored and reported in the returned positions as -1.
func (l *EventLog) Append(ctx context.Context, tenantID string, envelopes []contractsv1.Envelope) ([]LogPosition, error) {
	positions := make([]LogPosition, len(envelopes))
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		return l.appendBatch(ctx, tx, tenantID, envelopes, positions)
	}); err != nil {
		return nil, fmt.Errorf("append events: %w", err)
	}
	return positions, nil
}

func (l *EventLog) appendBatch(ctx context.Context, tx *sql.Tx, tenantID string, envelopes []contractsv1.Envelope, positions []LogPosition) error {
	for i, env := range envelopes {
		if err := l.admit(ctx, tx, tenantID, env); err != nil {
			return err
		}
		pos, err := l.appendOne(ctx, tx, tenantID, env)
		if err != nil {
			return err
		}
		positions[i] = pos
	}
	return nil
}

// admit validates the envelope contract and, when required, the registered
// event schema.
func (l *EventLog) admit(ctx context.Context, tx *sql.Tx, tenantID string, env contractsv1.Envelope) error {
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return fmt.Errorf("validate envelope: %w", err)
	}
	if !l.requireSchemas {
		return nil
	}
	return validateEnvelopeAgainstSchema(ctx, tx, env)
}

// appendOne inserts one envelope and returns its position, or -1 when the
// tenant already logged the event ID.
func (l *EventLog) appendOne(ctx context.Context, tx *sql.Tx, tenantID string, env contractsv1.Envelope) (LogPosition, error) {
	if _, err := contractsv1.ParseTraceContext(env.Traceparent, env.Tracestate); err != nil {
		return -1, fmt.Errorf("validate trace context: %w", err)
	}
	body, err := domain.EncodeEventBody(env.Data, env.Quality)
	if err != nil {
		return -1, err
	}
	return l.insertEnvelope(ctx, tx, tenantID, env, body, l.clk.Now().UTC().Format(time.RFC3339Nano))
}

func (l *EventLog) insertEnvelope(ctx context.Context, tx *sql.Tx, tenantID string, env contractsv1.Envelope, body domain.EncodedEvent, createdAt string) (LogPosition, error) {
	res, err := tx.ExecContext(ctx, insertEventSQL, eventColumns(tenantID, env, body, createdAt)...)
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

// eventColumns lists one envelope in insertEventSQL column order.
func eventColumns(tenantID string, env contractsv1.Envelope, body domain.EncodedEvent, createdAt string) []any {
	return []any{
		tenantID, env.PartitionID(0), env.ID, env.Type, env.SchemaVersion,
		env.Source, env.PartitionKey, env.Entity.Type, env.Entity.ID, env.EventTime.Format(time.RFC3339Nano),
		nullableTime(env.ObservedAt), env.IngestedAt.Format(time.RFC3339Nano), nullableText(env.CorrelationID), nullableText(env.CausationID),
		nullableText(env.Traceparent), nullableText(env.Tracestate), string(env.Classification), body.QualityJSON, body.PayloadJSON,
		body.PayloadSHA256, createdAt,
	}
}

// insertedPosition returns the new row's position, or -1 when the insert was
// ignored as a duplicate.
func insertedPosition(res sql.Result) (LogPosition, error) {
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

func nullableText(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: value.Format(time.RFC3339Nano), Valid: true}
}
