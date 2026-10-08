package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// LogPosition is the durable offset of a record in the event log.
type LogPosition int64

// ReadRequest selects a range of records from the log.
type ReadRequest struct {
	TenantID      string
	PartitionID   int
	AfterPosition LogPosition
	Limit         int
}

// EntityWindow is a read-only evidence window over one tenant's entity.
type EntityWindow struct {
	TenantID, EntityID string
	From, Until        time.Time
	MaxRows            uint64
}

// EntityEvent is one event of an entity evidence window.
type EntityEvent struct {
	EventID, EventType, EventTime string
	Payload                       []byte
}

// ScannedEvent is one event_log row as scanned, with its times parsed and its
// payload and quality documents still encoded.
type ScannedEvent struct {
	Position                                            LogPosition
	TenantID                                            string
	PartitionID                                         int
	EventID, EventType, SchemaVersion, Source           string
	PartitionKey, EntityType, EntityID                  string
	Classification                                      string
	EventTime, IngestedAt                               time.Time
	ObservedAt                                          *time.Time
	CorrelationID, CausationID, Traceparent, Tracestate *string
	QualityJSON, PayloadJSON                            []byte
}

// DecodePayload decodes one stored payload document.
func DecodePayload(payloadJSON []byte) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(payloadJSON, &data); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return data, nil
}
