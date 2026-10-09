package domain_test

import (
	"io/fs"
	"maps"
	"os"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

const (
	digest1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digest2 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	stamp   = "2026-08-04T22:26:00.000000000Z"
)

func validDocument(schema domain.SchemaName) map[string]any {
	documents := map[domain.SchemaName]map[string]any{
		domain.SchemaSnapshot: {
			"situation_id": "sit_1", "situation_version": 1, "situation_type": "test",
			"tenant_id": "default", "entity": map[string]any{"type": "motor", "id": "m1"},
			"phase": "watch", "severity": 10, "completeness": "on_time",
			"event_horizon": stamp, "spec_digest": digest1, "facts": map[string]any{"temperature_c": 4.2},
		},
		domain.SchemaDecision: {
			"decision_id": "dec_1", "episode_id": "epi_1", "attempt_id": "att_1", "fence": 1,
			"snapshot_digest": digest2, "confidence": 0.8, "decision_type": "need_more_evidence", "intents": []any{},
		},
		domain.SchemaIntent: {
			"intent_id": "int_1", "intent_digest": digest1, "decision_id": "dec_1", "tenant_id": "default",
			"situation_id": "sit_1", "situation_version": 1, "type": "maintenance.ticket",
			"risk_class": "R1", "parameters": map[string]any{}, "expires_at": stamp,
		},
		domain.SchemaCommand: {
			"command_id": "cmd_1", "intent_id": "int_1", "tenant_id": "default",
			"effector_route": "maintenance.ticket", "normalized_target": "motor/m1",
			"idempotency_key": digest1, "status": "prepared", "created_at": stamp,
		},
		domain.SchemaOutcome: {
			"outcome_id": "out_1", "command_id": "cmd_1", "status": "succeeded", "observed_at": stamp,
		},
		domain.SchemaTriggerEvaluation: {
			"trigger_id": "trg_1", "situation_id": "sit_1", "situation_version": 1, "outcome": "admitted",
		},
	}
	return maps.Clone(documents[schema])
}

var documentSchemas = []domain.SchemaName{
	domain.SchemaSnapshot, domain.SchemaDecision, domain.SchemaIntent, domain.SchemaCommand,
	domain.SchemaOutcome, domain.SchemaTriggerEvaluation,
}

var deviceSchemas = []domain.SchemaName{
	domain.SchemaDeviceCommand, domain.SchemaDeviceReceipt, domain.SchemaDeviceResult, domain.SchemaDeviceState,
}

func TestSchemaIDIsTheStableURNOfEveryKnownSchema(t *testing.T) {
	t.Parallel()
	for _, schema := range append(append([]domain.SchemaName{}, documentSchemas...), deviceSchemas...) {
		id, err := domain.SchemaID(schema)
		if want := "urn:situation-runtime:schema:" + string(schema) + ":v1"; err != nil || id != want {
			t.Errorf("SchemaID(%q) = %q, %v; want %q", schema, id, err, want)
		}
	}
}

func TestUnknownSchemaNamesAreRefused(t *testing.T) {
	t.Parallel()
	for _, name := range []domain.SchemaName{"", "nonsense", "Snapshot", "snapshot-v1"} {
		if id, err := domain.SchemaID(name); err == nil || id != "" || !strings.Contains(err.Error(), "unknown contract schema") {
			t.Errorf("SchemaID(%q) = %q, %v; want the unknown schema error", name, id, err)
		}
		if err := domain.Validate(name, map[string]any{}); err == nil || !strings.Contains(err.Error(), "unknown contract schema") {
			t.Errorf("Validate(%q) = %v; want the unknown schema error", name, err)
		}
	}
}

func TestEveryEmbeddedSchemaFileBelongsToAKnownSchema(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(os.DirFS("."), "schemas/v1")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, schema := range append(append([]domain.SchemaName{}, documentSchemas...), deviceSchemas...) {
		known[string(schema)+"-v1.json"] = true
	}
	for _, entry := range entries {
		if !known[entry.Name()] {
			t.Errorf("schemas/v1/%s has no SchemaName", entry.Name())
		}
		delete(known, entry.Name())
	}
	for name := range known {
		t.Errorf("SchemaName for schemas/v1/%s has no embedded file", name)
	}
}

func TestEverySchemaAcceptsItsValidDocumentAndRejectsUnknownProperties(t *testing.T) {
	t.Parallel()
	for _, schema := range documentSchemas {
		t.Run(string(schema), func(t *testing.T) {
			t.Parallel()
			if err := domain.Validate(schema, validDocument(schema)); err != nil {
				t.Fatalf("valid document rejected: %v", err)
			}
			extended := validDocument(schema)
			extended["unexpected"] = true
			if err := domain.Validate(schema, extended); err == nil || !strings.Contains(err.Error(), "validate "+string(schema)) {
				t.Fatalf("a document with an unknown property = %v, want a %s validation error", err, schema)
			}
			if err := domain.Validate(schema, "not an object"); err == nil {
				t.Fatal("a non-object document validated")
			}
		})
	}
}

func TestSchemasEnforceTheirRulesOnAMutatedDocument(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema domain.SchemaName
		mutate func(map[string]any)
		valid  bool
	}{
		{"snapshot version below 1", domain.SchemaSnapshot, func(d map[string]any) { d["situation_version"] = 0 }, false},
		{"snapshot unknown completeness", domain.SchemaSnapshot, func(d map[string]any) { d["completeness"] = "later" }, false},
		{"snapshot upper case spec digest", domain.SchemaSnapshot, func(d map[string]any) { d["spec_digest"] = strings.ToUpper(digest1) }, false},
		{"snapshot event horizon is not a date-time", domain.SchemaSnapshot, func(d map[string]any) { d["event_horizon"] = "yesterday" }, false},
		{"snapshot entity without id", domain.SchemaSnapshot, func(d map[string]any) { d["entity"] = map[string]any{"type": "motor"} }, false},
		{"snapshot confidence above 1", domain.SchemaSnapshot, func(d map[string]any) { d["confidence"] = 1.5 }, false},
		{"snapshot without facts", domain.SchemaSnapshot, func(d map[string]any) { delete(d, "facts") }, false},
		{"snapshot confidence at the bound", domain.SchemaSnapshot, func(d map[string]any) { d["confidence"] = 1 }, true},
		{"decision fence zero", domain.SchemaDecision, func(d map[string]any) { d["fence"] = 0 }, false},
		{"decision negative confidence", domain.SchemaDecision, func(d map[string]any) { d["confidence"] = -0.1 }, false},
		{"decision malformed snapshot digest", domain.SchemaDecision, func(d map[string]any) { d["snapshot_digest"] = "sha256:abc" }, false},
		{"decision with no intents must say why in decision_type", domain.SchemaDecision, func(d map[string]any) { delete(d, "decision_type") }, false},
		{"decision need_more_evidence with an intent", domain.SchemaDecision, func(d map[string]any) { d["intents"] = []any{map[string]any{}} }, false},
		{"decision with seventeen intents", domain.SchemaDecision, func(d map[string]any) {
			delete(d, "decision_type")
			d["intents"] = make([]any, 17)
			for i := range 17 {
				d["intents"].([]any)[i] = map[string]any{}
			}
		}, false},
		{"decision with an intent needs no decision_type", domain.SchemaDecision, func(d map[string]any) {
			delete(d, "decision_type")
			d["intents"] = []any{map[string]any{}}
		}, true},
		{"intent risk class outside R0..R4", domain.SchemaIntent, func(d map[string]any) { d["risk_class"] = "R5" }, false},
		{"intent type in upper case", domain.SchemaIntent, func(d map[string]any) { d["type"] = "Maintenance" }, false},
		{"intent duplicate evidence ids", domain.SchemaIntent, func(d map[string]any) { d["evidence_ids"] = []any{"e1", "e1"} }, false},
		{"intent expires_at is not a date-time", domain.SchemaIntent, func(d map[string]any) { d["expires_at"] = "soon" }, false},
		{"intent without digest", domain.SchemaIntent, func(d map[string]any) { delete(d, "intent_digest") }, false},
		{"intent with distinct evidence ids", domain.SchemaIntent, func(d map[string]any) { d["evidence_ids"] = []any{"e1", "e2"} }, true},
		{"command unknown status", domain.SchemaCommand, func(d map[string]any) { d["status"] = "done" }, false},
		{"command unprefixed idempotency key", domain.SchemaCommand, func(d map[string]any) { d["idempotency_key"] = strings.TrimPrefix(digest1, "sha256:") }, false},
		{"command outcome_unknown status", domain.SchemaCommand, func(d map[string]any) { d["status"] = "outcome_unknown" }, true},
		{"outcome status of a command", domain.SchemaOutcome, func(d map[string]any) { d["status"] = "prepared" }, false},
		{"outcome observed_at is not a date-time", domain.SchemaOutcome, func(d map[string]any) { d["observed_at"] = "2026-08-04" }, false},
		{"outcome reconciled", domain.SchemaOutcome, func(d map[string]any) { d["status"] = "reconciled" }, true},
		{"trigger evaluation unknown outcome", domain.SchemaTriggerEvaluation, func(d map[string]any) { d["outcome"] = "later" }, false},
		{"trigger evaluation version zero", domain.SchemaTriggerEvaluation, func(d map[string]any) { d["situation_version"] = 0 }, false},
		{"trigger evaluation deferred", domain.SchemaTriggerEvaluation, func(d map[string]any) { d["outcome"] = "deferred" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			document := validDocument(tt.schema)
			tt.mutate(document)
			err := domain.Validate(tt.schema, document)
			if tt.valid && err != nil {
				t.Fatalf("document refused: %v", err)
			}
			if !tt.valid && (err == nil || !strings.Contains(err.Error(), "validate "+string(tt.schema))) {
				t.Fatalf("document accepted or wrongly reported: %v", err)
			}
		})
	}
}
