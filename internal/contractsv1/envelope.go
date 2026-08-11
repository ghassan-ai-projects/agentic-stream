package contractsv1

import (
	"time"
)

// Classification levels for data handled by the runtime.
type Classification string

const (
	ClassificationPublic       Classification = "public"
	ClassificationInternal     Classification = "internal"
	ClassificationConfidential Classification = "confidential"
	ClassificationRestricted   Classification = "restricted"
)

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
	Classification Classification `json:"classification"`
	Quality        []QualityFlag  `json:"quality"`
	Data           map[string]any `json:"data"`
}

// PayloadHash is the SHA-256 digest of the original normalized payload.
type PayloadHash [32]byte
