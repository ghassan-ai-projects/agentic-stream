package contractsv1_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func TestFacadeValidatesEnvelopes(t *testing.T) {
	t.Parallel()
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
	if err := contractsv1.ValidateEnvelope(envelope, contractsv1.TenantID); err == nil || !strings.Contains(err.Error(), "event id") {
		t.Fatalf("envelope without an id = %v, want the event id error", err)
	}
}

func TestFacadeParsesTraceContext(t *testing.T) {
	t.Parallel()
	if _, err := contractsv1.ParseTraceContext("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", ""); err != nil {
		t.Fatalf("valid traceparent: %v", err)
	}
	if _, err := contractsv1.ParseTraceContext("garbage", ""); err == nil || !strings.Contains(err.Error(), "traceparent") {
		t.Fatalf("invalid traceparent = %v, want the traceparent error", err)
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

func TestFacadeDocumentFieldsProjectWithoutCoercion(t *testing.T) {
	t.Parallel()
	document := map[string]any{"name": "pump", "count": float64(7)}
	if contractsv1.DocumentString(document, "name") != "pump" || contractsv1.DocumentString(document, "count") != "" {
		t.Fatal("DocumentString must read strings only")
	}
	if contractsv1.DocumentInt(document, "count") != 7 || contractsv1.DocumentInt(document, "name") != 0 {
		t.Fatal("DocumentInt must read numbers only")
	}
}

func TestFacadeRiskPolicyAgreesWithTheRouteForEveryClass(t *testing.T) {
	t.Parallel()
	classes := []contractsv1.RiskClass{contractsv1.RiskR0, contractsv1.RiskR1, contractsv1.RiskR2, contractsv1.RiskR3, contractsv1.RiskR4}
	policy := contractsv1.RiskPolicyDocument()
	for _, class := range classes {
		if got := policy[string(class)]; got != string(contractsv1.RouteFor(class, false)) {
			t.Fatalf("policy document routes %s to %v, RouteFor says %v", class, got, contractsv1.RouteFor(class, false))
		}
	}
	if contractsv1.RouteFor(contractsv1.RiskR1, true) != contractsv1.RouteApproval || contractsv1.RouteFor("R9", false) != contractsv1.RouteDenied {
		t.Fatal("a flagged R1 needs approval and an unknown class is denied")
	}
	if len(contractsv1.IncompleteSourceHealthDocument()) == 0 {
		t.Fatal("the incomplete-source-health document must list the consequential classes")
	}
}

func TestFacadeVerifiesAStoredDocumentAgainstItsSchemaAndDigest(t *testing.T) {
	t.Parallel()
	command := contractstest.ValidFrame("command")
	canonical, sum, err := canonicaljson.Seal(canonicaljson.DomainCommand, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contractsv1.DecodeDocumentJSON(canonical); err != nil {
		t.Fatalf("DecodeDocumentJSON: %v", err)
	}
	if !contractsv1.VerifyDocumentDigest(canonicaljson.DomainCommand, command, sum) {
		t.Fatal("VerifyDocumentDigest refused the sum that seals the document")
	}
	tests := []struct {
		name string
		raw  []byte
		sum  []byte
		want error
	}{
		{"not unambiguous JSON", contractstest.AmbiguousKeyJSON(canonical, "command_id"), sum, contractsv1.ErrDocumentJSON},
		{"violates the schema", []byte(`{}`), sum, contractsv1.ErrDocumentSchema},
		{"digest of another document", canonical, make([]byte, 32), contractsv1.ErrDocumentDigest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaDeviceCommand, canonicaljson.DomainCommand, tt.raw, tt.sum); !errors.Is(err, tt.want) {
				t.Fatalf("VerifyStoredDocument = %v, want %v", err, tt.want)
			}
		})
	}
	if _, err := contractsv1.DecodeDocument([]byte(`{}`), contractsv1.SchemaDeviceCommand); !errors.Is(err, contractsv1.ErrDocumentSchema) {
		t.Fatalf("DecodeDocument of an empty document = %v, want ErrDocumentSchema", err)
	}
}
