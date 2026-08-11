package contractsv1

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
	for name, value := range map[string]string{
		"id": e.ID, "source": e.Source, "type": e.Type, "dataschema": e.DataSchema,
		"tenantid": e.TenantID, "partitionkey": e.PartitionKey,
	} {
		if value == "" {
			return fmt.Errorf("cloud event: %s is required", name)
		}
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
	projection := map[string]any{
		"specversion":    e.SpecVersion,
		"type":           e.Type,
		"source":         e.Source,
		"id":             e.ID,
		"subject":        e.Subject,
		"time":           e.Time.UTC().Format(time.RFC3339Nano),
		"dataschema":     e.DataSchema,
		"tenantid":       e.TenantID,
		"partitionkey":   e.PartitionKey,
		"classification": e.Classification,
		"datadigest":     dataDigest,
	}
	return canonicaljson.Digest(canonicaljson.DomainEnvelope, projection)
}
