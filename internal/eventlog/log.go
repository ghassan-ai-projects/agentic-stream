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

// EventLog appends and reads normalized events.
type EventLog struct {
	db             *storage.DB
	clk            clock.Clock
	requireSchemas bool
}

// CurrentPosition returns the greatest durable log position for tenantID.
// It returns zero when the tenant has no records.
func (l *EventLog) CurrentPosition(ctx context.Context, tenantID string) (LogPosition, error) {
	if l == nil || l.db == nil {
		return 0, fmt.Errorf("event log storage is required")
	}
	var position int64
	if err := l.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(position), 0) FROM event_log WHERE tenant_id = ?", tenantID,
	).Scan(&position); err != nil {
		return 0, fmt.Errorf("read current event position for tenant %q: %w", tenantID, err)
	}
	return LogPosition(position), nil
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
