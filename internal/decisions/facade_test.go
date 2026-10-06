package decisions

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestValidateDelegatesAndPreservesTypedRefusal(t *testing.T) {
	t.Parallel()
	_, err := Validate(nil, "", Input{})
	var refusal *ValidationError
	if !errors.As(err, &refusal) || refusal.Reason != "schema_invalid" || refusal.Details["field"] != "clock" {
		t.Fatalf("validation refusal = %v, want missing trusted clock", err)
	}
}

func TestCompileIntentCatalogDelegates(t *testing.T) {
	t.Parallel()
	if _, err := CompileIntentCatalog(nil); err == nil {
		t.Fatal("empty intent catalog was accepted")
	}
}

func TestFacadeValidatesAgainstCopiedCatalogAuthority(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	source := facadeCatalog()
	catalog, err := CompileIntentCatalog(source)
	if err != nil {
		t.Fatal(err)
	}
	defaultPreset := source[0]["presets"].(map[string]any)["default"].(map[string]any)
	defaultPreset["priority"] = "attacker"
	defaultPreset["metadata"].(map[string]any)["labels"].([]any)[0] = "attacker"
	source[0]["parameter_schema"].(map[string]any)["properties"].(map[string]any)["priority"] = map[string]any{"type": "integer"}
	input := facadeInput(catalog, now)
	decision := facadeDecision(t, now)
	raw, digest := facadeDecisionBytes(t, decision)
	result, err := Validate(raw, digest, input)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	canonicalIntent, err := canonicaljson.Marshal(decision["intents"].([]any)[0])
	if err != nil {
		t.Fatal(err)
	}
	if result.DecisionID != "decision-1" || len(result.Intents) != 1 || result.Intents[0].ID != "intent-1" || !bytes.Equal(result.Intents[0].CanonicalJSON, canonicalIntent) {
		t.Fatalf("validated result = %#v", result)
	}
	attackerDecision := facadeDecision(t, now)
	attackerIntent := attackerDecision["intents"].([]any)[0].(map[string]any)
	attackerIntent["parameters"].(map[string]any)["metadata"].(map[string]any)["labels"].([]any)[0] = "attacker"
	delete(attackerIntent, "intent_digest")
	intentDigest, err := contractsv1.IntentDigest(attackerIntent)
	if err != nil {
		t.Fatal(err)
	}
	attackerIntent["intent_digest"] = intentDigest
	attackerRaw, attackerDigest := facadeDecisionBytes(t, attackerDecision)
	_, err = Validate(attackerRaw, attackerDigest, input)
	var refusal *ValidationError
	if !errors.As(err, &refusal) || refusal.Reason != "preset_mismatch" {
		t.Fatalf("mutated preset proposal refusal = %v, want preset_mismatch", err)
	}
}

func facadeDecisionBytes(t *testing.T, document map[string]any) ([]byte, string) {
	t.Helper()
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatal(err)
	}
	return raw, digest
}

func facadeCatalog() []map[string]any {
	return []map[string]any{{
		"type": "act", "risk_class": "R1",
		"parameter_schema": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"priority": map[string]any{"type": "string"},
				"metadata": map[string]any{
					"type": "object", "additionalProperties": false,
					"properties": map[string]any{"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
				},
			},
		},
		"presets": map[string]any{"default": map[string]any{
			"priority": "routine", "metadata": map[string]any{"labels": []any{"original"}},
		}},
	}}
}

func facadeInput(catalog *IntentCatalog, now time.Time) Input {
	return Input{
		EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1,
		TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1,
		EntityID: "entity-1", SnapshotDigest: "sha256:" + facadeZeros(64),
		AllowedIntentTypes: map[string]struct{}{"act": {}}, RiskCeiling: "R1",
		IntentCatalog: catalog, Kind: "standard", Now: now,
	}
}

func facadeDecision(t *testing.T, now time.Time) map[string]any {
	t.Helper()
	intent := map[string]any{
		"intent_id": "intent-1", "decision_id": "decision-1", "tenant_id": "tenant-1",
		"situation_id": "situation-1", "situation_version": 1, "type": "act",
		"risk_class": "R1", "parameters": map[string]any{
			"priority": "routine", "metadata": map[string]any{"labels": []any{"original"}},
		},
		"expires_at": now.Add(time.Hour).Format(time.RFC3339Nano),
	}
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	intent["intent_digest"] = digest
	return map[string]any{
		"decision_id": "decision-1", "episode_id": "episode-1", "attempt_id": "attempt-1",
		"fence": 1, "snapshot_digest": "sha256:" + facadeZeros(64),
		"situation_id": "situation-1", "situation_version": 1, "confidence": 0.8,
		"summary": "act on the bound Situation", "intents": []any{intent},
	}
}

func facadeZeros(length int) string {
	return string(bytes.Repeat([]byte{'0'}, length))
}
