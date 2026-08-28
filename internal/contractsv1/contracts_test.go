package contractsv1_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestSharedSchemaIDsAndValidation(t *testing.T) {
	tests := []struct {
		name   string
		schema contractsv1.SchemaName
		valid  map[string]any
	}{
		{
			name:   "snapshot",
			schema: contractsv1.SchemaSnapshot,
			valid: map[string]any{
				"situation_id": "sit_1", "situation_version": 1, "situation_type": "test",
				"tenant_id": "default", "entity": map[string]any{"type": "motor", "id": "m1"},
				"phase": "watch", "severity": 10, "completeness": "on_time",
				"event_horizon": "2026-08-04T22:26:00.000000000Z",
				"spec_digest":   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				"facts":         map[string]any{"temperature_c": 4.2},
			},
		},
		{
			name:   "decision",
			schema: contractsv1.SchemaDecision,
			valid: map[string]any{
				"decision_id": "dec_1", "episode_id": "epi_1",
				"attempt_id": "att_1", "fence": 1,
				"snapshot_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
				"confidence":      0.8, "decision_type": "need_more_evidence", "intents": []any{},
			},
		},
		{
			name:   "intent",
			schema: contractsv1.SchemaIntent,
			valid: map[string]any{
				"intent_id": "int_1", "intent_digest": "sha256:4444444444444444444444444444444444444444444444444444444444444444", "decision_id": "dec_1", "tenant_id": "default",
				"situation_id": "sit_1", "situation_version": 1, "type": "maintenance.ticket",
				"risk_class": "R1", "parameters": map[string]any{}, "expires_at": "2026-08-04T23:26:00.000000000Z",
			},
		},
		{
			name:   "command",
			schema: contractsv1.SchemaCommand,
			valid: map[string]any{
				"command_id": "cmd_1", "intent_id": "int_1", "tenant_id": "default",
				"effector_route": "maintenance.ticket", "normalized_target": "motor/m1",
				"idempotency_key": "sha256:3333333333333333333333333333333333333333333333333333333333333333",
				"status":          "prepared", "created_at": "2026-08-04T23:26:00.000000000Z",
			},
		},
		{
			name:   "outcome",
			schema: contractsv1.SchemaOutcome,
			valid: map[string]any{
				"outcome_id": "out_1", "command_id": "cmd_1", "status": "succeeded",
				"observed_at": "2026-08-04T23:27:00.000000000Z",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, err := contractsv1.SchemaID(test.schema)
			if err != nil {
				t.Fatalf("SchemaID: %v", err)
			}
			if id != "urn:situation-runtime:schema:"+string(test.schema)+":v1" {
				t.Fatalf("schema id = %q", id)
			}
			if err := contractsv1.Validate(test.schema, test.valid); err != nil {
				t.Fatalf("valid document rejected: %v", err)
			}
		})
	}
}

func TestSharedSchemaRejectsUnknownProperties(t *testing.T) {
	valid := map[string]any{
		"decision_id": "dec_1", "episode_id": "epi_1",
		"attempt_id": "att_1", "fence": 1,
		"snapshot_digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"confidence":      0.8, "decision_type": "need_more_evidence", "intents": []any{}, "unexpected": true,
	}
	if err := contractsv1.Validate(contractsv1.SchemaDecision, valid); err == nil {
		t.Fatal("expected unknown decision property to be rejected")
	}
}

func TestCloudEventEnvelopeDigestBindsMetadataAndData(t *testing.T) {
	now := time.Date(2026, 8, 4, 22, 26, 0, 0, time.UTC)
	event := contractsv1.CloudEvent{
		SpecVersion:     "1.0",
		ID:              "evt_notification_1",
		Source:          "//agentic-stream/tenants/default/situations",
		Type:            "io.agenticstream.situation.version.published.v1",
		Subject:         "situation/sit_1",
		Time:            now,
		DataContentType: "application/json",
		DataSchema:      "urn:situation-runtime:schema:snapshot:v1",
		Data:            map[string]any{"version": 1},
		TenantID:        "default",
		PartitionKey:    "motor-1",
		IngestedTime:    now.Add(time.Second),
		Classification:  contractsv1.ClassificationInternal,
	}
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		t.Fatalf("ComputeEnvelopeDigest: %v", err)
	}
	event.EnvelopeDigest = digest
	if err := event.Validate(); err != nil {
		t.Fatalf("valid CloudEvent rejected: %v", err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainEnvelope, map[string]any{
		"specversion": "1.0", "type": event.Type, "source": event.Source, "id": event.ID,
		"subject": event.Subject, "time": now.Format(time.RFC3339Nano),
		"dataschema": event.DataSchema, "tenantid": "default", "partitionkey": "motor-1",
		"classification": "internal", "datadigest": mustDigest(t, event.Data),
	}, digest) {
		t.Fatal("envelope digest did not verify against its projection")
	}
	event.Subject = "situation/other"
	if err := event.Validate(); err == nil {
		t.Fatal("metadata mutation should invalidate the envelope digest")
	}
}

func TestTraceContextValidationAndSpanLink(t *testing.T) {
	ctx, err := contractsv1.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "vendor=value")
	if err != nil {
		t.Fatalf("parse trace context: %v", err)
	}
	link := ctx.Link()
	if link == nil || link.Traceparent != ctx.Traceparent || link.Tracestate != ctx.Tracestate {
		t.Fatalf("span link did not preserve trace context: %#v", link)
	}
	for _, tc := range []struct {
		name, parent, state string
	}{
		{"zero trace id", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", ""},
		{"zero span id", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", ""},
		{"state without parent", "", "vendor=value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := contractsv1.ParseTraceContext(tc.parent, tc.state); err == nil {
				t.Fatal("expected invalid trace context")
			}
		})
	}
}

func mustDigest(t *testing.T, value any) string {
	t.Helper()
	digest, err := canonicaljson.Digest(canonicaljson.DomainEvent, value)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return digest
}
