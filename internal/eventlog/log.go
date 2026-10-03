// Package eventlog provides the append-only normalized evidence log.
package eventlog

import (
	"context"
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
