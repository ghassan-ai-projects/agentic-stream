package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// LifecycleEvent is the request to publish one contract-validated lifecycle
// notification, preserving the source W3C trace context. The payload fixes the
// event type.
type LifecycleEvent struct {
	ID           string
	TenantID     string
	Subject      string
	PartitionKey string
	Payload      Payload
	At           time.Time
	Trace        contractsv1.TraceContext
}

// NewLifecycleEvent builds the sealed, contract-validated CloudEvent for the
// request. It fails before building when the identity or trace is invalid.
func NewLifecycleEvent(request LifecycleEvent) (contractsv1.CloudEvent, error) {
	if err := request.checkIdentity(); err != nil {
		return contractsv1.CloudEvent{}, err
	}
	data, err := eventData(request.Payload, request.TenantID)
	if err != nil {
		return contractsv1.CloudEvent{}, err
	}
	event := request.cloudEvent(data)
	if event.EnvelopeDigest, err = event.ComputeEnvelopeDigest(); err != nil {
		return contractsv1.CloudEvent{}, fmt.Errorf("compute lifecycle event digest: %w", err)
	}
	if err := ValidateLifecycle(event); err != nil {
		return contractsv1.CloudEvent{}, fmt.Errorf("validate lifecycle contract: %w", err)
	}
	return event, nil
}

func (request LifecycleEvent) checkIdentity() error {
	if request.ID == "" || request.TenantID == "" || request.Subject == "" || request.PartitionKey == "" || request.Payload == nil {
		return fmt.Errorf("lifecycle event identity is incomplete")
	}
	if _, err := contractsv1.ParseTraceContext(request.Trace.Traceparent, request.Trace.Tracestate); err != nil {
		return fmt.Errorf("lifecycle trace context: %w", err)
	}
	return nil
}

func (request LifecycleEvent) cloudEvent(data map[string]any) contractsv1.CloudEvent {
	at := request.At.UTC()
	return contractsv1.CloudEvent{
		SpecVersion: "1.0", ID: request.ID, Source: SourceForTenant(request.TenantID), Type: request.Payload.EventType(), Subject: request.Subject,
		Time: at, DataContentType: "application/json", DataSchema: SchemaID, Data: data,
		TenantID: request.TenantID, PartitionKey: request.PartitionKey, IngestedTime: at, Classification: contractsv1.ClassificationInternal,
		Traceparent: request.Trace.Traceparent, Tracestate: request.Trace.Tracestate,
	}
}
