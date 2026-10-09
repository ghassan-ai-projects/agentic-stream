package domain_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

var eventTime = time.Date(2026, 8, 4, 22, 26, 0, 0, time.UTC)

func sealedCloudEvent(t *testing.T, edit func(*domain.CloudEvent)) domain.CloudEvent {
	t.Helper()
	event := domain.CloudEvent{
		SpecVersion:     "1.0",
		ID:              "evt_notification_1",
		Source:          "//agentic-stream/tenants/default/situations",
		Type:            "io.agenticstream.situation.version.published.v1",
		Subject:         "situation/sit_1",
		Time:            eventTime,
		DataContentType: "application/json",
		DataSchema:      "urn:situation-runtime:schema:snapshot:v1",
		Data:            map[string]any{"version": 1},
		TenantID:        "default",
		PartitionKey:    "motor-1",
		IngestedTime:    eventTime.Add(time.Second),
		Classification:  domain.ClassificationInternal,
	}
	edit(&event)
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		t.Fatalf("ComputeEnvelopeDigest: %v", err)
	}
	event.EnvelopeDigest = digest
	return event
}

func keep(*domain.CloudEvent) {}

func TestCloudEventEnvelopeDigestIsTheDigestOfItsSecurityProjection(t *testing.T) {
	t.Parallel()
	event := sealedCloudEvent(t, keep)
	if err := event.Validate(); err != nil {
		t.Fatalf("valid CloudEvent rejected: %v", err)
	}
	dataDigest, err := canonicaljson.Digest(canonicaljson.DomainEvent, event.Data)
	if err != nil {
		t.Fatal(err)
	}
	projection := map[string]any{
		"specversion": "1.0", "type": event.Type, "source": event.Source, "id": event.ID,
		"subject": event.Subject, "time": "2026-08-04T22:26:00Z",
		"dataschema": event.DataSchema, "tenantid": "default", "partitionkey": "motor-1",
		"classification": "internal", "datadigest": dataDigest,
	}
	if !canonicaljson.Verify(canonicaljson.DomainEnvelope, projection, event.EnvelopeDigest) {
		t.Fatal("envelope digest did not verify against its projection")
	}
}

func TestCloudEventEnvelopeDigestGolden(t *testing.T) {
	t.Parallel()
	const want = "sha256:64db0858d28c6486cf4ec638cf25c2461f7dd8a34d1ad14e633be4695cb07346"
	if got := sealedCloudEvent(t, keep).EnvelopeDigest; got != want {
		t.Fatalf("envelope digest = %s, want the frozen %s", got, want)
	}
}

func TestCloudEventEnvelopeDigestBindsEveryProjectedAttributeAndTheData(t *testing.T) {
	t.Parallel()
	base := sealedCloudEvent(t, keep).EnvelopeDigest
	tests := []struct {
		name string
		edit func(*domain.CloudEvent)
	}{
		{"subject", func(e *domain.CloudEvent) { e.Subject = "situation/other" }},
		{"source", func(e *domain.CloudEvent) { e.Source = "//other" }},
		{"type", func(e *domain.CloudEvent) { e.Type = "io.other.v1" }},
		{"id", func(e *domain.CloudEvent) { e.ID = "evt_other" }},
		{"time", func(e *domain.CloudEvent) { e.Time = e.Time.Add(time.Second) }},
		{"data schema", func(e *domain.CloudEvent) { e.DataSchema = "urn:other" }},
		{"tenant", func(e *domain.CloudEvent) { e.TenantID = "other" }},
		{"partition key", func(e *domain.CloudEvent) { e.PartitionKey = "motor-2" }},
		{"classification", func(e *domain.CloudEvent) { e.Classification = "restricted" }},
		{"data", func(e *domain.CloudEvent) { e.Data = map[string]any{"version": 2} }},
		{"trace parent", func(e *domain.CloudEvent) { e.Traceparent = validTraceparent }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sealedCloudEvent(t, tt.edit).EnvelopeDigest; got == base {
				t.Fatalf("changing the %s kept the envelope digest %s", tt.name, base)
			}
		})
	}
	t.Run("time is bound at wire precision in UTC", func(t *testing.T) {
		t.Parallel()
		zoned := sealedCloudEvent(t, func(e *domain.CloudEvent) { e.Time = e.Time.In(time.FixedZone("plus2", 2*3600)) })
		if zoned.EnvelopeDigest != base {
			t.Fatalf("the same instant in another zone changed the digest: %s != %s", zoned.EnvelopeDigest, base)
		}
	})
}

func TestCloudEventValidateRefusesAMutationAfterSealing(t *testing.T) {
	t.Parallel()
	event := sealedCloudEvent(t, keep)
	event.Subject = "situation/other"
	if err := event.Validate(); err == nil || !strings.Contains(err.Error(), "envelopedigest mismatch") {
		t.Fatalf("Validate after a metadata mutation = %v, want the digest mismatch", err)
	}
	event = sealedCloudEvent(t, keep)
	event.Data = map[string]any{"version": 2}
	if err := event.Validate(); err == nil || !strings.Contains(err.Error(), "envelopedigest mismatch") {
		t.Fatalf("Validate after a data mutation = %v, want the digest mismatch", err)
	}
}

func TestCloudEventValidateRequiresTheCloudEventsAttributes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*domain.CloudEvent)
		want string
	}{
		{"specversion", func(e *domain.CloudEvent) { e.SpecVersion = "0.3" }, "specversion must be 1.0"},
		{"id", func(e *domain.CloudEvent) { e.ID = "" }, "id is required"},
		{"source", func(e *domain.CloudEvent) { e.Source = "" }, "source is required"},
		{"type", func(e *domain.CloudEvent) { e.Type = "" }, "type is required"},
		{"dataschema", func(e *domain.CloudEvent) { e.DataSchema = "" }, "dataschema is required"},
		{"tenantid", func(e *domain.CloudEvent) { e.TenantID = "" }, "tenantid is required"},
		{"partitionkey", func(e *domain.CloudEvent) { e.PartitionKey = "" }, "partitionkey is required"},
		{"time", func(e *domain.CloudEvent) { e.Time = time.Time{} }, "time and ingestedtime are required"},
		{"ingestedtime", func(e *domain.CloudEvent) { e.IngestedTime = time.Time{} }, "time and ingestedtime are required"},
		{"datacontenttype", func(e *domain.CloudEvent) { e.DataContentType = "text/plain" }, "datacontenttype must be application/json"},
		{"trace context", func(e *domain.CloudEvent) { e.Traceparent = "garbage" }, "trace context"},
		{"tracestate without traceparent", func(e *domain.CloudEvent) { e.Tracestate = "vendor=value" }, "trace context"},
		{"unencodable data", func(e *domain.CloudEvent) { e.Data = math.NaN() }, "compute envelopedigest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			event := sealedCloudEventUnchecked(tt.edit)
			if err := event.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestCloudEventValidateRequiresTheEnvelopeDigest(t *testing.T) {
	t.Parallel()
	event := sealedCloudEvent(t, keep)
	event.EnvelopeDigest = ""
	if err := event.Validate(); err == nil || !strings.Contains(err.Error(), "envelopedigest is required") {
		t.Fatalf("Validate without a digest = %v", err)
	}
}

func TestCloudEventWithTraceContextValidates(t *testing.T) {
	t.Parallel()
	event := sealedCloudEvent(t, func(e *domain.CloudEvent) {
		e.Traceparent, e.Tracestate = validTraceparent, "vendor=value"
	})
	if err := event.Validate(); err != nil {
		t.Fatalf("CloudEvent with a trace context refused: %v", err)
	}
}

func sealedCloudEventUnchecked(edit func(*domain.CloudEvent)) domain.CloudEvent {
	event := domain.CloudEvent{
		SpecVersion: "1.0", ID: "evt_1", Source: "//s", Type: "t", Time: eventTime, DataContentType: "application/json",
		DataSchema: "urn:s", Data: map[string]any{}, TenantID: "default", PartitionKey: "k",
		IngestedTime: eventTime, EnvelopeDigest: "sha256:x", Classification: domain.ClassificationInternal,
	}
	edit(&event)
	return event
}
