package domain

import (
	"errors"
	"strings"
	"testing"
)

const validLine = `{"id":"evt-1","type":"motor.vibration.observed","schema_version":"1.0","source":"sim","partition_key":"m1","entity":{"type":"motor","id":"m1"},"event_time":"2026-01-01T00:00:00Z","ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}`

func TestAdmitEnvelopeGivesALineWithoutATenantTheConnectorTenant(t *testing.T) {
	t.Parallel()
	verdict := AdmitEnvelope([]byte(validLine), "acme")
	if verdict.Rejected() || verdict.Envelope.TenantID != "acme" {
		t.Fatalf("verdict = %+v, want an admitted envelope of tenant acme", verdict)
	}
}

func TestAdmitEnvelopeRejectsWithTheReasonAndWhetherTheLineDecoded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		line        string
		wantReason  string
		wantDecoded bool
	}{
		{name: "not json", line: "not-json", wantReason: ReasonMalformedJSON},
		{name: "another tenant", line: strings.Replace(validLine, `"source"`, `"tenant_id":"other","source"`, 1), wantReason: ReasonEnvelopeInvalid, wantDecoded: true},
		{name: "missing envelope field", line: strings.Replace(validLine, `"partition_key":"m1",`, "", 1), wantReason: ReasonEnvelopeInvalid, wantDecoded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			verdict := AdmitEnvelope([]byte(tt.line), "acme")
			if !verdict.Rejected() || verdict.Reason != tt.wantReason || verdict.Decoded != tt.wantDecoded || verdict.Cause == nil {
				t.Fatalf("verdict = %+v, want %s (decoded %t) with a cause", verdict, tt.wantReason, tt.wantDecoded)
			}
		})
	}
}

func TestSchemaRejectedKeepsTheDecodedEnvelopeAndItsCause(t *testing.T) {
	t.Parallel()
	admitted := AdmitEnvelope([]byte(validLine), "acme")
	cause := errors.New("no schema")

	verdict := SchemaRejected(admitted.Envelope, cause)

	if verdict.Reason != ReasonSchemaInvalid || !verdict.Decoded || !verdict.Rejected() || verdict.Envelope.ID != "evt-1" || !errors.Is(verdict.Cause, cause) {
		t.Fatalf("verdict = %+v", verdict)
	}
}
