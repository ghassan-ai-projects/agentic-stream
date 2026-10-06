package contractsv1_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestFacadeValidatesEnvelopesAndKeepsContractVersions(t *testing.T) {
	t.Parallel()
	if contractsv1.ContractVersion == "" || contractsv1.ProtocolVersion == "" || contractsv1.TenantID == "" {
		t.Fatal("contract versions and default tenant must be defined")
	}
	envelope := contractsv1.Envelope{
		ID: "evt_1", Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: contractsv1.TenantID,
		Source: "test", PartitionKey: "k", Entity: contractsv1.EntityRef{Type: "motor", ID: "m1"},
		EventTime: time.Unix(1, 0).UTC(), IngestedAt: time.Unix(2, 0).UTC(),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{},
	}
	if err := contractsv1.ValidateEnvelope(envelope, contractsv1.TenantID); err != nil {
		t.Fatalf("valid envelope: %v", err)
	}
	envelope.ID = ""
	if err := contractsv1.ValidateEnvelope(envelope, contractsv1.TenantID); err == nil {
		t.Fatal("envelope without an id accepted")
	}
}

func TestFacadeParsesTraceContext(t *testing.T) {
	t.Parallel()
	if _, err := contractsv1.ParseTraceContext("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", ""); err != nil {
		t.Fatalf("valid traceparent: %v", err)
	}
	if _, err := contractsv1.ParseTraceContext("garbage", ""); err == nil {
		t.Fatal("invalid traceparent accepted")
	}
}

func TestFacadeSchemasAndIntentDigests(t *testing.T) {
	t.Parallel()
	name, ok := contractsv1.SchemaForMessageType("command")
	if !ok {
		t.Fatal("command message type has no schema")
	}
	if err := contractsv1.Validate(name, map[string]any{}); err == nil {
		t.Fatal("empty command document validated")
	}
	if _, ok := contractsv1.SchemaForMessageType("nonsense"); ok {
		t.Fatal("unknown message type mapped to a schema")
	}
	document := map[string]any{"intent_id": "int_1", "type": "ticket"}
	if _, err := contractsv1.IntentDigest(document); err != nil {
		t.Fatalf("intent digest: %v", err)
	}
	if contractsv1.VerifyIntentDigest(document) {
		t.Fatal("document without a digest verified")
	}
}
