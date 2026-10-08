package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// CloudEvent is the JSON CloudEvents 1.0 notification envelope used at
// product boundaries. Runtime-only processing and decision timestamps are not
// part of this type.
type CloudEvent struct {
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Source          string         `json:"source"`
	Type            string         `json:"type"`
	Subject         string         `json:"subject,omitempty"`
	Time            time.Time      `json:"time"`
	DataContentType string         `json:"datacontenttype"`
	DataSchema      string         `json:"dataschema"`
	Data            any            `json:"data"`
	TenantID        string         `json:"tenantid"`
	PartitionKey    string         `json:"partitionkey"`
	ObservedTime    *time.Time     `json:"observedtime,omitempty"`
	IngestedTime    time.Time      `json:"ingestedtime"`
	EnvelopeDigest  string         `json:"envelopedigest"`
	Traceparent     string         `json:"traceparent,omitempty"`
	Tracestate      string         `json:"tracestate,omitempty"`
	Classification  Classification `json:"classification"`
}

// Validate checks the required CloudEvents attributes and the runtime
// extensions. It does not validate Data; callers must validate Data against
// the schema identified by DataSchema before publishing.
func (e CloudEvent) Validate() error {
	if e.SpecVersion != "1.0" {
		return fmt.Errorf("cloud event: specversion must be 1.0")
	}
	if err := e.checkRequiredAttributes(); err != nil {
		return err
	}
	if _, err := ParseTraceContext(e.Traceparent, e.Tracestate); err != nil {
		return fmt.Errorf("cloud event: trace context: %w", err)
	}
	return e.checkEnvelopeDigest()
}

// checkRequiredAttributes requires the identity attributes, both times,
// JSON content and an envelope digest.
func (e CloudEvent) checkRequiredAttributes() error {
	if err := e.checkIdentityAttributes(); err != nil {
		return err
	}
	if e.Time.IsZero() || e.IngestedTime.IsZero() {
		return fmt.Errorf("cloud event: time and ingestedtime are required")
	}
	if e.DataContentType != "application/json" {
		return fmt.Errorf("cloud event: datacontenttype must be application/json")
	}
	if e.EnvelopeDigest == "" {
		return fmt.Errorf("cloud event: envelopedigest is required")
	}
	return nil
}

func (e CloudEvent) checkIdentityAttributes() error {
	for name, value := range map[string]string{
		"id": e.ID, "source": e.Source, "type": e.Type, "dataschema": e.DataSchema,
		"tenantid": e.TenantID, "partitionkey": e.PartitionKey,
	} {
		if value == "" {
			return fmt.Errorf("cloud event: %s is required", name)
		}
	}
	return nil
}

func (e CloudEvent) checkEnvelopeDigest() error {
	expected, err := e.ComputeEnvelopeDigest()
	if err != nil {
		return fmt.Errorf("cloud event: compute envelopedigest: %w", err)
	}
	if expected != e.EnvelopeDigest {
		return fmt.Errorf("cloud event: envelopedigest mismatch")
	}
	return nil
}

// ComputeEnvelopeDigest computes the digest over the CloudEvents security
// projection. The data itself is represented by its event-payload digest so
// changing data or envelope metadata changes the final digest.
func (e CloudEvent) ComputeEnvelopeDigest() (string, error) {
	dataDigest, err := canonicaljson.Digest(canonicaljson.DomainEvent, e.Data)
	if err != nil {
		return "", fmt.Errorf("data digest: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainEnvelope, e.envelopeProjection(dataDigest))
	if err != nil {
		return "", fmt.Errorf("digest cloud event envelope: %w", err)
	}
	return digest, nil
}

// envelopeProjection is the digested view of the envelope: its attributes,
// the data digest and, when present, the trace context.
func (e CloudEvent) envelopeProjection(dataDigest string) map[string]any {
	projection := map[string]any{
		"specversion": e.SpecVersion, "type": e.Type, "source": e.Source, "id": e.ID,
		"subject": e.Subject, "time": sources.FormatTime(e.Time),
		"dataschema": e.DataSchema, "tenantid": e.TenantID, "partitionkey": e.PartitionKey,
		"classification": e.Classification, "datadigest": dataDigest,
	}
	if e.Traceparent != "" {
		projection["traceparent"] = e.Traceparent
		projection["tracestate"] = e.Tracestate
	}
	return projection
}
