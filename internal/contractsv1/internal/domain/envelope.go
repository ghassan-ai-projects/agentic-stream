package domain

import (
	"fmt"
	"hash/fnv"
	"time"
)

// Classification is the data-handling level of an envelope. Spec inputs
// declare it as data; the runtime itself writes only the internal level.
type Classification string

// ClassificationInternal is the level of every envelope the runtime writes.
const ClassificationInternal Classification = "internal"

// QualityFlag captures data-quality annotations on an event.
type QualityFlag struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// EntityRef identifies the domain entity an event belongs to.
type EntityRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Envelope is the normalized event representation inside the runtime.
// Field order matches the external JSON contract in docs/design/TECHNICAL_DESIGN.md.
type Envelope struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	SchemaVersion  string         `json:"schema_version"`
	TenantID       string         `json:"tenant_id"`
	Source         string         `json:"source"`
	PartitionKey   string         `json:"partition_key"`
	Entity         EntityRef      `json:"entity"`
	EventTime      time.Time      `json:"event_time"`
	ObservedAt     *time.Time     `json:"observed_at,omitempty"`
	IngestedAt     time.Time      `json:"ingested_at"`
	CorrelationID  string         `json:"correlation_id,omitempty"`
	CausationID    string         `json:"causation_id,omitempty"`
	Traceparent    string         `json:"traceparent,omitempty"`
	Tracestate     string         `json:"tracestate,omitempty"`
	Classification Classification `json:"classification"`
	Quality        []QualityFlag  `json:"quality"`
	Data           map[string]any `json:"data"`
}

// PayloadHash is the SHA-256 digest of the original normalized payload.
type PayloadHash [32]byte

// ValidateEnvelope checks the required invariants of the normalized ingress
// contract before an envelope enters the durable event log.
func ValidateEnvelope(e Envelope, tenantID string) error {
	if err := validateEnvelopeIdentity(e, tenantID); err != nil {
		return err
	}
	if e.EventTime.IsZero() || e.IngestedAt.IsZero() {
		return fmt.Errorf("event_time and ingested_at are required")
	}
	if e.ObservedAt != nil && e.ObservedAt.Before(e.EventTime) {
		return fmt.Errorf("observed_at must not precede event_time")
	}
	if e.Data == nil {
		return fmt.Errorf("data is required")
	}
	return nil
}

// validateEnvelopeIdentity requires the event, tenant, partition and entity
// identity, with the tenant matching the runtime's when one is given.
func validateEnvelopeIdentity(e Envelope, tenantID string) error {
	if e.ID == "" || e.Type == "" || e.SchemaVersion == "" || e.Source == "" {
		return fmt.Errorf("event id, type, schema_version, and source are required")
	}
	if e.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if tenantID != "" && e.TenantID != tenantID {
		return fmt.Errorf("tenant mismatch: envelope=%q runtime=%q", e.TenantID, tenantID)
	}
	if e.PartitionKey == "" || e.Entity.Type == "" || e.Entity.ID == "" {
		return fmt.Errorf("partition_key and entity identity are required")
	}
	return nil
}

// PartitionID computes the stable virtual partition for this envelope.
func (e Envelope) PartitionID(count int) int {
	if count <= 0 {
		count = PartitionCount
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(e.TenantID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(e.PartitionKey))
	return int(h.Sum64() % uint64(count)) //nolint:gosec // the result is below count, a positive int
}
