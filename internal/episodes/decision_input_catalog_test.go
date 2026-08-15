package episodes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}

func zeros(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = '0'
	}
	return string(result)
}

func deepCopy(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var copy map[string]any
	_ = json.Unmarshal(raw, &copy)
	return copy
}

// P4 exit gate 5: the decision-validation input fails closed when the request
// payload carries a missing, forged, or malformed intent catalog — the
// executor's digest verification is the independent boundary (B10), never
// trusting the worker's own verify_wire.
func TestDecisionInputRejectsCatalogAttacks(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	intentCatalog, intentDigest, err := CompileIntentCatalog([]spec.Intent{
		{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	validPayload := map[string]any{
		"allowed_intent_types": []string{"create_ticket"},
		"risk_ceiling":         "R1",
		"executor": map[string]any{
			"intent_catalog":        intentCatalog,
			"intent_catalog_sha256": intentDigest,
		},
	}
	identity := Identity{EpisodeID: "epi", AttemptID: "att", Fence: 1}

	// The honest payload compiles.
	raw, err := json.Marshal(validPayload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := &Request{RequestJSON: raw, TenantID: "tenant", SituationID: "sit",
		SituationVersion: 1, SnapshotSHA256: "sha256:" + zeros(64)}
	if _, err := decisionInput(req, identity, now); err != nil {
		t.Fatalf("honest payload must compile: %v", err)
	}

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "missing catalog", payload: map[string]any{
			"allowed_intent_types": []string{"create_ticket"}, "risk_ceiling": "R1",
		}},
		{name: "forged digest", payload: func() map[string]any {
			payload := deepCopy(validPayload)
			payload["executor"].(map[string]any)["intent_catalog_sha256"] = "sha256:" + zeros(64)
			return payload
		}()},
		{name: "tampered bytes with original digest", payload: func() map[string]any {
			payload := deepCopy(validPayload)
			executor := payload["executor"].(map[string]any)
			executor["intent_catalog"] = []map[string]any{{
				"type": "create_ticket", "risk_class": "R0",
				"parameter_schema": map[string]any{"type": "object"},
			}}
			return payload
		}()},
		{name: "malformed catalog", payload: func() map[string]any {
			payload := deepCopy(validPayload)
			executor := payload["executor"].(map[string]any)
			executor["intent_catalog"] = []map[string]any{}
			executor["intent_catalog_sha256"], _ = canonicaljson.Digest(
				canonicaljson.DomainIntentCatalog, []map[string]any{})
			return payload
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			req := &Request{RequestJSON: raw, TenantID: "tenant", SituationID: "sit",
				SituationVersion: 1, SnapshotSHA256: "sha256:" + zeros(64)}
			if _, err := decisionInput(req, identity, now); err == nil {
				t.Fatalf("decisionInput must fail closed for %s", test.name)
			}
		})
	}
}
