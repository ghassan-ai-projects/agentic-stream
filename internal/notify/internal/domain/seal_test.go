package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

var sealNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func sampleEvent(id string, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/t", Type: "situation.version.published", Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": 1}, TenantID: "t", PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	event.EnvelopeDigest, _ = event.ComputeEnvelopeDigest()
	return event
}

func TestSealCanonicalizesAndHashesAdmittedEvents(t *testing.T) {
	t.Parallel()
	first, err := Seal(sampleEvent("a", sealNow), sealNow)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Seal(sampleEvent("a", sealNow), sealNow)
	if err != nil || string(first.SHA) != string(again.SHA) || len(first.SHA) != 32 || len(first.JSON) == 0 {
		t.Fatalf("seal not deterministic: %v", err)
	}
}

func TestSealRefusesInvalidAndExpiredEvents(t *testing.T) {
	t.Parallel()
	invalid := sampleEvent("a", sealNow)
	invalid.EnvelopeDigest = ""
	if _, err := Seal(invalid, sealNow); err == nil || !strings.Contains(err.Error(), "validate notification") {
		t.Fatalf("invalid event error = %v", err)
	}
	old := sampleEvent("a", sealNow.Add(-RetentionFloor-time.Second))
	if _, err := Seal(old, sealNow); !errors.Is(err, ErrEventExpired) {
		t.Fatalf("expired event error = %v", err)
	}
	edge := sampleEvent("a", sealNow.Add(-RetentionFloor))
	if _, err := Seal(edge, sealNow); err != nil {
		t.Fatalf("event at the horizon was refused: %v", err)
	}
}

func TestSamePayloadAndRetiredDecisions(t *testing.T) {
	t.Parallel()
	same, other := []byte{1}, []byte{2}
	if err := CheckSamePayload("e", same, same); err != nil {
		t.Fatalf("identical payload refused: %v", err)
	}
	if err := CheckSamePayload("e", same, other); err == nil || !strings.Contains(err.Error(), "conflicting payload") {
		t.Fatalf("conflict error = %v", err)
	}
	if err := RefuseRetired("e", same, other); err == nil || !strings.Contains(err.Error(), "conflicts with tombstone") {
		t.Fatalf("tombstone conflict error = %v", err)
	}
	if err := RefuseRetired("e", same, same); err == nil || !strings.Contains(err.Error(), "already retired") {
		t.Fatalf("retired error = %v", err)
	}
}
