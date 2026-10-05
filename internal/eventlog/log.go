// Package eventlog provides the append-only normalized evidence log.
package eventlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// errStorageRequired means the log was used without its database.
var errStorageRequired = errors.New("event log storage is required")

// LogPosition is the durable offset of a record in the event log.
type LogPosition = domain.LogPosition

// ReadRequest selects a range of records from the log.
type ReadRequest = domain.ReadRequest

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

// EventLog appends and reads normalized events. It delegates to the module's
// use cases; SQL lives in the store layer and pure rules in the domain layer.
type EventLog struct {
	service *app.Service
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
	return &EventLog{service: app.New(clk, store.New(db))}
}

// CurrentPosition returns the greatest durable log position for tenantID.
// It returns zero when the tenant has no records.
func (l *EventLog) CurrentPosition(ctx context.Context, tenantID string) (LogPosition, error) {
	if l == nil || l.service == nil {
		return 0, errStorageRequired
	}
	return l.service.CurrentPosition(ctx, tenantID)
}

// RequireSchemaValidation makes ingress validate every envelope against the
// durable event_schemas registry before it can enter event_log.
func (l *EventLog) RequireSchemaValidation() *EventLog {
	l.service.RequireSchemaValidation()
	return l
}

// ValidateEnvelope checks an envelope against the durable registered schema.
func (l *EventLog) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	return l.service.ValidateEnvelope(ctx, env)
}

// Append inserts envelopes into the event log. Duplicate event IDs for the same
// tenant are ignored and reported in the returned positions as -1.
func (l *EventLog) Append(ctx context.Context, tenantID string, envelopes []contractsv1.Envelope) ([]LogPosition, error) {
	return l.service.Append(ctx, tenantID, envelopes)
}

// Read streams records matching req into callback.
func (l *EventLog) Read(ctx context.Context, req ReadRequest, callback func(Record) error) error {
	return l.service.Read(ctx, req, func(scanned domain.ScannedEvent) error {
		record, err := recordFromScanned(scanned)
		if err != nil {
			return err
		}
		return callback(record)
	})
}

// recordFromScanned decodes one scanned row into the public record with its
// rebuilt envelope contract, in the stored column order: times, then quality,
// then payload.
func recordFromScanned(scanned domain.ScannedEvent) (Record, error) {
	decoded, err := scanned.Decode()
	if err != nil {
		return Record{}, err
	}
	quality, err := qualityFlags(scanned.QualityJSON)
	if err != nil {
		return Record{}, err
	}
	data, err := domain.DecodePayload(scanned.PayloadJSON)
	if err != nil {
		return Record{}, err
	}
	return recordFromDecoded(decoded, envelopeFromDecoded(decoded, quality, data)), nil
}

func recordFromDecoded(d domain.DecodedEvent, envelope contractsv1.Envelope) Record {
	return Record{
		Position: d.Position, TenantID: d.TenantID, PartitionID: d.PartitionID,
		EventID: d.EventID, EventType: d.EventType, SchemaVersion: d.SchemaVersion,
		Source: d.Source, PartitionKey: d.PartitionKey, EntityType: d.EntityType,
		EntityID: d.EntityID, EventTime: d.ParsedEventTime, ObservedAt: d.ParsedObservedAt,
		IngestedAt: d.ParsedIngestedAt, Envelope: envelope,
	}
}

func qualityFlags(qualityJSON []byte) ([]contractsv1.QualityFlag, error) {
	var quality []contractsv1.QualityFlag
	if err := json.Unmarshal(qualityJSON, &quality); err != nil {
		return nil, fmt.Errorf("unmarshal quality: %w", err)
	}
	return quality, nil
}

// envelopeFromDecoded rebuilds the normalized envelope contract from one
// decoded row.
func envelopeFromDecoded(d domain.DecodedEvent, quality []contractsv1.QualityFlag, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: d.EventID, Type: d.EventType, SchemaVersion: d.SchemaVersion, TenantID: d.TenantID,
		Source: d.Source, PartitionKey: d.PartitionKey,
		Entity:    contractsv1.EntityRef{Type: d.EntityType, ID: d.EntityID},
		EventTime: d.ParsedEventTime, ObservedAt: d.ParsedObservedAt, IngestedAt: d.ParsedIngestedAt,
		CorrelationID: textOf(d.CorrelationID), CausationID: textOf(d.CausationID),
		Traceparent: textOf(d.Traceparent), Tracestate: textOf(d.Tracestate),
		Classification: contractsv1.Classification(d.Classification),
		Quality:        quality, Data: data,
	}
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
