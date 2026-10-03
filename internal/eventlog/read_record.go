package eventlog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

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
