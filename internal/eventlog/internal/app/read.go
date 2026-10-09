package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// Record is one stored event-log row with its normalized envelope.
type Record struct {
	Position      domain.LogPosition
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

// Read streams decoded records matching req into visit in log order.
func (s *Service) Read(ctx context.Context, req domain.ReadRequest, visit func(Record) error) error {
	return s.store.ReadRecords(ctx, req, func(scanned domain.ScannedEvent) error {
		record, err := recordFromScanned(scanned)
		if err != nil {
			return err
		}
		return visit(record)
	}) //nolint:wrapcheck // Store owns the query error context.
}

func recordFromScanned(scanned domain.ScannedEvent) (Record, error) {
	quality, err := qualityFlags(scanned.QualityJSON)
	if err != nil {
		return Record{}, err
	}
	data, err := domain.DecodePayload(scanned.PayloadJSON)
	if err != nil {
		return Record{}, err
	}
	return newRecord(scanned, envelopeFromScanned(scanned, quality, data)), nil
}

func newRecord(d domain.ScannedEvent, envelope contractsv1.Envelope) Record {
	return Record{
		Position: d.Position, TenantID: d.TenantID, PartitionID: d.PartitionID,
		EventID: d.EventID, EventType: d.EventType, SchemaVersion: d.SchemaVersion,
		Source: d.Source, PartitionKey: d.PartitionKey, EntityType: d.EntityType,
		EntityID: d.EntityID, EventTime: d.EventTime, ObservedAt: d.ObservedAt,
		IngestedAt: d.IngestedAt, Envelope: envelope,
	}
}

func qualityFlags(qualityJSON []byte) ([]contractsv1.QualityFlag, error) {
	var quality []contractsv1.QualityFlag
	if err := json.Unmarshal(qualityJSON, &quality); err != nil {
		return nil, fmt.Errorf("unmarshal quality: %w", err)
	}
	return quality, nil
}

func envelopeFromScanned(d domain.ScannedEvent, quality []contractsv1.QualityFlag, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: d.EventID, Type: d.EventType, SchemaVersion: d.SchemaVersion, TenantID: d.TenantID,
		Source: d.Source, PartitionKey: d.PartitionKey,
		Entity:    contractsv1.EntityRef{Type: d.EntityType, ID: d.EntityID},
		EventTime: d.EventTime, ObservedAt: d.ObservedAt, IngestedAt: d.IngestedAt,
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
