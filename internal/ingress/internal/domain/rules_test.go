package domain

import (
	"errors"
	"strings"
	"testing"
)

const validLine = `{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","source":"sim","partition_key":"m1","entity":{"type":"motor","id":"m1"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}`

func TestAdmitEnvelopeAppliesTheTenantAndRefusesInOrder(t *testing.T) {
	t.Parallel()
	admitted := AdmitEnvelope([]byte(validLine), "acme")
	if admitted.Rejected() || admitted.Envelope.TenantID != "acme" {
		t.Fatalf("a line without a tenant must take the connector tenant: %+v", admitted)
	}
	malformed := AdmitEnvelope([]byte("not-json"), "acme")
	if malformed.Reason != ReasonMalformedJSON || malformed.Decoded || malformed.Cause == nil {
		t.Fatalf("malformed verdict = %+v", malformed)
	}
	wrongTenant := AdmitEnvelope([]byte(strings.Replace(validLine, `"source"`, `"tenant_id":"other","source"`, 1)), "acme")
	if wrongTenant.Reason != ReasonEnvelopeInvalid || !wrongTenant.Decoded {
		t.Fatalf("a foreign tenant must be refused as an invalid envelope: %+v", wrongTenant)
	}
	schema := SchemaRejected(admitted.Envelope, errors.New("no schema"))
	if schema.Reason != ReasonSchemaInvalid || !schema.Decoded || !schema.Rejected() {
		t.Fatalf("schema verdict = %+v", schema)
	}
}

func TestIdentitiesScopeQuarantineToTheirSource(t *testing.T) {
	t.Parallel()
	if QuarantineID("replay:a", 3) != "replay:a:line:3" || QuarantineID("replay:b", 3) == QuarantineID("replay:a", 3) {
		t.Fatal("quarantine identity must be connector scoped")
	}
	if LiveQuarantineID("inst", 2, 9) != "live-uds:inst:2:9" {
		t.Fatal("live quarantine identity changed")
	}
	if JSONLConnectorID("", "/t.jsonl") != "jsonl:/t.jsonl" || JSONLConnectorID("x", "/t.jsonl") != "x" ||
		SimulatorConnectorID("", "/t.jsonl") != "simulator-jsonl:/t.jsonl" || SimulatorConnectorID("y", "/t.jsonl") != "y" {
		t.Fatal("connector identity defaults changed")
	}
	if TenantOrDefault("") != DefaultTenant || TenantOrDefault("acme") != "acme" {
		t.Fatal("tenant default changed")
	}
}

func TestCheckpointRoundTripsAndRefusesCorruption(t *testing.T) {
	t.Parallel()
	blob, err := EncodeCheckpoint(42)
	if err != nil || string(blob) != `{"version":1,"last_line":42}` {
		t.Fatalf("blob = %s err=%v", blob, err)
	}
	if line, err := DecodeCheckpoint(blob); err != nil || line != 42 {
		t.Fatalf("line = %d err=%v", line, err)
	}
	if _, err := DecodeCheckpoint([]byte("{")); err == nil {
		t.Fatal("a corrupt checkpoint decoded")
	}
}

func TestSocketPathMustBeCleanAbsoluteAndLocal(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"/tmp/live.sock", "/run/agentic/live.sock"} {
		if err := ValidateSocketPath(ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "live.sock", "/tmp/../live.sock", "/tmp//live.sock", "unix:///tmp/live.sock", "/tmp/live\x00.sock"} {
		if ValidateSocketPath(bad) == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}
