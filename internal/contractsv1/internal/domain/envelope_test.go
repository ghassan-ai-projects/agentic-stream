package domain

import (
	"strings"
	"testing"
	"time"
)

func validEnvelope() Envelope {
	eventTime := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	return Envelope{
		ID: "evt-1", Type: "sensor.reading", SchemaVersion: "1", TenantID: "tenant-a", Source: "sim",
		PartitionKey: "motor-1", Entity: EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: eventTime, IngestedAt: eventTime.Add(time.Second), Data: map[string]any{"value": 1},
	}
}

func TestValidateEnvelope(t *testing.T) {
	t.Parallel()

	before := validEnvelope().EventTime.Add(-time.Second)
	tests := []struct {
		name    string
		mutate  func(*Envelope)
		tenant  string
		wantErr string
	}{
		{name: "valid", mutate: func(*Envelope) {}, tenant: "tenant-a"},
		{name: "any tenant when runtime is unscoped", mutate: func(*Envelope) {}},
		{name: "missing id", mutate: func(e *Envelope) { e.ID = "" }, wantErr: "event id"},
		{name: "missing tenant", mutate: func(e *Envelope) { e.TenantID = "" }, wantErr: "tenant_id is required"},
		{name: "tenant mismatch", mutate: func(*Envelope) {}, tenant: "tenant-b", wantErr: "tenant mismatch"},
		{name: "missing entity", mutate: func(e *Envelope) { e.Entity.ID = "" }, wantErr: "entity identity"},
		{name: "missing event time", mutate: func(e *Envelope) { e.EventTime = time.Time{} }, wantErr: "event_time"},
		{name: "observed before event", mutate: func(e *Envelope) { e.ObservedAt = &before }, wantErr: "observed_at"},
		{name: "missing data", mutate: func(e *Envelope) { e.Data = nil }, wantErr: "data is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			envelope := validEnvelope()
			tt.mutate(&envelope)
			err := ValidateEnvelope(envelope, tt.tenant)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestPartitionIDIsStableAndBounded(t *testing.T) {
	t.Parallel()

	envelope := validEnvelope()
	first := envelope.PartitionID(0)
	if first < 0 || first >= PartitionCount {
		t.Fatalf("default partition %d is outside [0, %d)", first, PartitionCount)
	}
	if again := envelope.PartitionID(PartitionCount); again != first {
		t.Fatalf("explicit PartitionCount gave %d, default gave %d", again, first)
	}
	if got := envelope.PartitionID(1); got != 0 {
		t.Fatalf("single partition = %d", got)
	}
	other := envelope
	other.TenantID = "tenant-b"
	seen := map[int]bool{}
	for _, key := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		other.PartitionKey = key
		seen[other.PartitionID(PartitionCount)] = true
	}
	if len(seen) < 2 {
		t.Fatal("partition keys did not spread across partitions")
	}
}

func TestIntentDigestExcludesItselfAndVerifies(t *testing.T) {
	t.Parallel()

	document := map[string]any{"intent_id": "int-1", "type": "inspect", "parameters": map[string]any{"depth": 2}}
	digest, err := IntentDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	document["intent_digest"] = digest
	if again, err := IntentDigest(document); err != nil || again != digest {
		t.Fatalf("digest must ignore its own field: %q, %v", again, err)
	}
	if !VerifyIntentDigest(document) {
		t.Fatal("valid digest did not verify")
	}

	tampered := map[string]any{"intent_id": "int-1", "type": "shutdown", "parameters": map[string]any{"depth": 2}, "intent_digest": digest}
	if VerifyIntentDigest(tampered) {
		t.Fatal("tampered intent verified")
	}
	if VerifyIntentDigest(map[string]any{"intent_id": "int-1"}) {
		t.Fatal("intent without a digest verified")
	}
	if VerifyIntentDigest(map[string]any{"intent_digest": 42}) {
		t.Fatal("non-string digest verified")
	}
}

func TestSchemaLoaderDeniesNetwork(t *testing.T) {
	t.Parallel()

	if _, err := (denyNetworkLoader{}).Load("https://example.com/schema.json"); err == nil {
		t.Fatal("external schema load was allowed")
	}
}
