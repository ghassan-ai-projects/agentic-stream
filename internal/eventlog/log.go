// Package eventlog provides the append-only normalized evidence log.
package eventlog

import (
	"context"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// LogPosition is the durable offset of a record in the event log.
type LogPosition = domain.LogPosition

// ReadRequest selects a range of records from the log.
type ReadRequest = domain.ReadRequest

// Record is one stored event-log row.
type Record = app.Record

// errStorageRequired means the log was used without its database.
var errStorageRequired = errors.New("event log storage is required")

// EventLog appends and reads normalized events. It delegates to the module's
// use cases; SQL lives in the store layer and pure rules in the domain layer.
type EventLog struct {
	service *app.Service
}

// NewEventLog creates an EventLog backed by db and the physical clock.
func NewEventLog(db *storage.DB) *EventLog {
	return NewEventLogWithClock(db, sources.Physical())
}

// NewEventLogWithClock creates an EventLog backed by db and the given clock.
func NewEventLogWithClock(db *storage.DB, clk sources.Clock) *EventLog {
	if clk == nil {
		clk = sources.Physical()
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
	return l.service.Read(ctx, req, callback)
}
