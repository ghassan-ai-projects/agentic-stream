package domain

import "testing"

func identityCatalog() []map[string]any {
	return []map[string]any{{
		"type": "act", "risk_class": "R1",
		"parameter_schema": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"priority":   map[string]any{"type": "string"},
				"entity_id":  map[string]any{"type": "string"},
				"target":     map[string]any{"type": "string"},
				"expires_at": map[string]any{"type": "string"},
			},
		},
		"presets":               map[string]any{"default": map[string]any{"priority": "routine"}},
		"model_writable_fields": []any{"entity_id", "target", "expires_at"},
	}}
}

func actDecision(parameters map[string]any) map[string]any {
	document := validDecision()
	document["facts_used"] = []any{map[string]any{"evidence": "fact:dissolved_oxygen"}}
	document["valid_until"] = "2026-08-12T10:30:00.000000000Z"
	intent := firstIntent(document)
	intent["type"] = "act"
	intent["parameters"] = parameters
	resealIntents(document)
	return document
}

func actInput(t *testing.T) Input {
	t.Helper()
	input := inputAllowing(t, "act")
	input.IntentCatalog = compileCatalog(t, identityCatalog())
	input.EntityID = "motor-7"
	return input
}

func TestValidateBindsIdentityParametersToTheEpisode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]any
		entityID   string
		wantField  string
	}{
		{"entity of the episode", map[string]any{"entity_id": "motor-7"}, "motor-7", ""},
		{"target of the episode", map[string]any{"target": "motor-7"}, "motor-7", ""},
		{"expiry equal to the decision validity", map[string]any{"expires_at": "2026-08-12T10:30:00.000000000Z"}, "motor-7", ""},
		{"another entity", map[string]any{"entity_id": "motor-9"}, "motor-7", "intent.parameters.entity_id"},
		{"another target", map[string]any{"target": "attacker-controlled-target"}, "motor-7", "intent.parameters.target"},
		{"entity without an episode identity", map[string]any{"entity_id": "motor-7"}, "", "intent.parameters.entity_id"},
		{"target without an episode identity", map[string]any{"target": "motor-7"}, "", "intent.parameters.target"},
		{"expiry beyond the decision validity", map[string]any{"expires_at": "2026-08-12T23:00:00.000000000Z"}, "motor-7", "intent.parameters.expires_at"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := actInput(t)
			input.EntityID = test.entityID
			_, err := validateDocument(t, actDecision(test.parameters), input)
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			requireRejection(t, err, "snapshot_mismatch", test.wantField)
		})
	}
}

func TestValidateKeepsPresetFieldsPresetAuthored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]any
		wantReject bool
	}{
		{"preset value unchanged", map[string]any{"priority": "routine"}, false},
		{"preset value substituted", map[string]any{"priority": "urgent"}, true},
		{"preset field left out", map[string]any{}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateDocument(t, actDecision(test.parameters), actInput(t))
			if test.wantReject {
				requireRejection(t, err, "preset_mismatch", "intent.parameters.priority")
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateLetsTheModelOverrideOnlyWritableFields(t *testing.T) {
	t.Parallel()
	entries := identityCatalog()
	entries[0]["model_writable_fields"] = []any{"priority"}
	input := actInput(t)
	input.IntentCatalog = compileCatalog(t, entries)
	_, err := validateDocument(t, actDecision(map[string]any{"priority": "urgent"}), input)
	if err != nil {
		t.Fatalf("writable override rejected: %v", err)
	}
}

func TestValidateRefusesParametersOutsideTheCatalogSchema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]any
	}{
		{"wrong type", map[string]any{"target": 42}},
		{"unknown property", map[string]any{"unlisted": "value"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateDocument(t, actDecision(test.parameters), actInput(t))
			requireRejection(t, err, "parameter_schema_violation", "intent.parameters")
		})
	}
}

func TestValidateGroundsIntentEvidenceInTheDecisionFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		evidence   []any
		wantReject bool
	}{
		{"grounded reference", []any{"fact:dissolved_oxygen"}, false},
		{"no references", nil, false},
		{"forged reference", []any{"fact:forged"}, true},
		{"one forged among grounded", []any{"fact:dissolved_oxygen", "fact:forged"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := actDecision(map[string]any{})
			if test.evidence != nil {
				firstIntent(document)["evidence_ids"] = test.evidence
				resealIntents(document)
			}
			_, err := validateDocument(t, document, actInput(t))
			if test.wantReject {
				requireRejection(t, err, "ungrounded_evidence", "intent.evidence_ids")
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
