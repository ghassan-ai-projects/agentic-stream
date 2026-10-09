package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const traceLine = `{"id":"evt-1","type":"zone.temp.observed","schema_version":"1.0","tenant_id":"default","source":"fixture","partition_key":"zone-01","entity":{"type":"thermal_zone","id":"zone-01"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"celsius":25}}`

func TestTraceEnvelopeDecodesOnlyWellFormedLines(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{"empty line": "", "malformed JSON": "not-json"} {
		if _, ok := TraceEnvelope([]byte(line)); ok {
			t.Errorf("%s was decoded as an envelope", name)
		}
	}
	envelope, ok := TraceEnvelope([]byte(traceLine))
	if !ok || envelope.ID != "evt-1" || envelope.TenantID != "default" {
		t.Fatalf("envelope = %+v ok=%v", envelope, ok)
	}
}

func TestAdoptTenantFillsOnlyAnEmptyTenant(t *testing.T) {
	t.Parallel()
	if got := AdoptTenant(contractsv1.Envelope{}, "default").TenantID; got != "default" {
		t.Fatalf("empty tenant adopted %q, want default", got)
	}
	if got := AdoptTenant(contractsv1.Envelope{TenantID: "other"}, "default").TenantID; got != "other" {
		t.Fatalf("an explicit tenant was replaced by %q", got)
	}
}

func TestContractValidEnvelopeRequiresTheReplayTenantAndAFullEnvelope(t *testing.T) {
	t.Parallel()
	envelope, ok := TraceEnvelope([]byte(traceLine))
	if !ok {
		t.Fatal("fixture line did not decode")
	}
	if !ContractValidEnvelope(envelope, "default") {
		t.Fatal("a complete envelope of the replay tenant was rejected")
	}
	if ContractValidEnvelope(envelope, "another-tenant") {
		t.Fatal("an envelope of another tenant was accepted")
	}
	if ContractValidEnvelope(contractsv1.Envelope{TenantID: "default"}, "default") {
		t.Fatal("an empty envelope was accepted")
	}
}
