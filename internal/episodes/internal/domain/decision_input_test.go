package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var decisionInputNow = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

func catalogTicketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}

func honestDecisionPayload(t *testing.T) map[string]any {
	t.Helper()
	intentCatalog, intentDigest, err := CompileIntentCatalog([]spec.Intent{
		{Type: "create_ticket", Risk: "R1", ParameterSchema: catalogTicketSchema()},
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	return map[string]any{
		"allowed_intent_types": []any{"create_ticket"},
		"risk_ceiling":         "R1",
		"executor": map[string]any{
			"intent_catalog":        toDocuments(t, intentCatalog),
			"intent_catalog_sha256": intentDigest,
		},
	}
}

func toDocuments(t *testing.T, catalog []map[string]any) []any {
	t.Helper()
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var documents []any
	if err := json.Unmarshal(raw, &documents); err != nil {
		t.Fatal(err)
	}
	return documents
}

func decisionInputFor(t *testing.T, payload map[string]any) error {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, err = DecisionInput(decisionRequest(raw), episodeledger.Identity{EpisodeID: "epi", AttemptID: "att", Fence: 1}, decisionInputNow)
	return err
}

func decisionRequest(raw []byte) *Request {
	return &Request{RequestJSON: raw, TenantID: "tenant", SituationID: "sit", EntityID: "motor-1",
		SituationVersion: 1, SnapshotSHA256: "sha256:" + strings.Repeat("0", 64)}
}

func TestDecisionInputBindsTheAttemptAndTheRequestAuthority(t *testing.T) {
	t.Parallel()
	payload := honestDecisionPayload(t)
	payload["kind"] = episodeledger.KindReconsider
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	identity := episodeledger.Identity{EpisodeID: "epi", AttemptID: "att", Fence: 4}
	input, err := DecisionInput(decisionRequest(raw), identity, decisionInputNow)
	if err != nil {
		t.Fatalf("honest payload must compile: %v", err)
	}
	_, allowed := input.AllowedIntentTypes["create_ticket"]
	if input.EpisodeID != "epi" || input.AttemptID != "att" || input.Fence != 4 || input.TenantID != "tenant" ||
		input.SituationID != "sit" || input.SituationVersion != 1 || input.EntityID != "motor-1" ||
		input.RiskCeiling != "R1" || !input.Reconsider || !allowed || len(input.AllowedIntentTypes) != 1 || !input.Now.Equal(decisionInputNow) {
		t.Fatalf("decision input = %+v", input)
	}
}

func TestDecisionInputFailsClosedOnAnUntrustedIntentCatalog(t *testing.T) {
	t.Parallel()
	emptyDigest, err := canonicaljson.Digest(canonicaljson.DomainIntentCatalog, []map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	executor := func(payload map[string]any) map[string]any { return payload["executor"].(map[string]any) }
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"missing catalog", func(p map[string]any) { delete(p, "executor") }, "missing, forged, or malformed"},
		{"forged digest", func(p map[string]any) {
			executor(p)["intent_catalog_sha256"] = "sha256:" + strings.Repeat("0", 64)
		}, "missing, forged, or malformed"},
		{"tampered bytes with original digest", func(p map[string]any) {
			executor(p)["intent_catalog"] = []any{map[string]any{
				"type": "create_ticket", "risk_class": "R0", "parameter_schema": map[string]any{"type": "object"},
			}}
		}, "missing, forged, or malformed"},
		{"empty catalog with its own digest", func(p map[string]any) {
			executor(p)["intent_catalog"] = []any{}
			executor(p)["intent_catalog_sha256"] = emptyDigest
		}, "compile intent catalog"},
		{"no risk ceiling", func(p map[string]any) { delete(p, "risk_ceiling") }, "no explicit risk ceiling"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := honestDecisionPayload(t)
			tc.mutate(payload)
			if err := decisionInputFor(t, payload); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestDecisionInputRefusesAnUndecodableRequest(t *testing.T) {
	t.Parallel()
	_, err := DecisionInput(decisionRequest([]byte("{")), episodeledger.Identity{}, decisionInputNow)
	if err == nil || !strings.HasPrefix(err.Error(), "decode request tools:") {
		t.Fatalf("error = %v, want decode request tools", err)
	}
}
