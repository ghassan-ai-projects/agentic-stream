package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func checkIntentParameters(document map[string]any, input Input, decisionDocument map[string]any, entry *intentEntry) error {
	// P4: parameters must satisfy the catalog's per-intent schema.
	parameters, _ := document["parameters"].(map[string]any)
	if err := entry.ParameterSchema.Validate(parameters); err != nil {
		return reject("parameter_schema_violation", "intent.parameters", err.Error())
	}
	if err := checkIdentityParameters(parameters, input, decisionDocument); err != nil {
		return err
	}
	// P4: preset-only fields must be byte-identical to the compiled preset —
	// an attempt to silently substitute a preset value is rejected, not
	// repaired.
	if err := verifyPresetEquality(document, entry); err != nil {
		return err
	}
	// P4: the intent's evidence must be grounded in the decision's facts.
	if err := verifyEvidenceBinding(document, decisionDocument); err != nil {
		return err
	}
	return nil
}

// checkIdentityParameters binds identity-bearing parameters to the dispatched
// episode (P4): a tampered entity_id would otherwise flow into the command
// payload unverified, since the preset check only covers preset keys.
func checkIdentityParameters(parameters map[string]any, input Input, decisionDocument map[string]any) error {
	if err := bindEntityParameter(parameters, input, "entity_id", "intent entity does not match the dispatched episode"); err != nil {
		return err
	}
	if err := bindExpiryParameter(parameters, decisionDocument); err != nil {
		return err
	}
	return bindEntityParameter(parameters, input, "target", "intent target must equal the bound entity")
}

func bindEntityParameter(parameters map[string]any, input Input, key, mismatch string) error {
	value, present := parameters[key]
	if !present {
		return nil
	}
	if input.EntityID == "" {
		return reject("snapshot_mismatch", "intent.parameters."+key, "the validator has no entity identity to bind against")
	}
	if value != input.EntityID {
		return reject("snapshot_mismatch", "intent.parameters."+key, mismatch)
	}
	return nil
}

func bindExpiryParameter(parameters, decisionDocument map[string]any) error {
	if expiresAt, present := parameters["expires_at"]; present {
		if validUntil, ok := decisionDocument["valid_until"].(string); ok && expiresAt != validUntil {
			return reject("snapshot_mismatch", "intent.parameters.expires_at", "intent parameter expires_at must equal the decision's valid_until")
		}
	}
	return nil
}

// verifyPresetEquality rejects any parameter whose key is NOT model-writable
// and IS present in the catalog's default preset but whose value differs from
// the preset's — the worker may only override the fields the catalog marks
// writable. Unknown keys (not in the preset, not writable) are schema-level
// noise and fail the schema check already.
func verifyPresetEquality(document map[string]any, entry *intentEntry) error {
	parameters, _ := document["parameters"].(map[string]any)
	defaultPreset := entry.Presets["default"]
	for key, presetValue := range defaultPreset {
		if entry.ModelWritable[key] {
			continue
		}
		intentValue, present := parameters[key]
		if !present {
			continue
		}
		if err := comparePresetParameter(key, presetValue, intentValue); err != nil {
			return err
		}
	}
	return nil
}

func comparePresetParameter(key string, presetValue, intentValue any) error {
	presetCanonical, err := canonicaljson.Marshal(presetValue)
	if err != nil {
		return reject("preset_mismatch", "intent.parameters."+key, err.Error())
	}
	intentCanonical, err := canonicaljson.Marshal(intentValue)
	if err != nil {
		return reject("preset_mismatch", "intent.parameters."+key, err.Error())
	}
	if string(presetCanonical) != string(intentCanonical) {
		return reject("preset_mismatch", "intent.parameters."+key,
			fmt.Sprintf("field %s is preset-authored, not model-writable", key))
	}
	return nil
}

// verifyEvidenceBinding grounds the intent's evidence_ids in the decision's
// facts_used refs (P4/T3).
func verifyEvidenceBinding(document, decisionDocument map[string]any) error {
	evidenceIDs, _ := document["evidence_ids"].([]any)
	if len(evidenceIDs) == 0 {
		return nil
	}
	grounded := factEvidenceRefs(decisionDocument)
	for _, ref := range evidenceIDs {
		if err := checkEvidenceReference(ref, grounded); err != nil {
			return err
		}
	}
	return nil
}

func checkEvidenceReference(ref any, grounded map[string]bool) error {
	text, ok := ref.(string)
	if !ok {
		return reject("parameter_schema_violation", "intent.evidence_ids", "evidence id must be a string")
	}
	if !grounded[text] {
		return reject("ungrounded_evidence", "intent.evidence_ids",
			fmt.Sprintf("evidence id %s is not among the decision's facts_used", text))
	}
	return nil
}

// factEvidenceRefs collects the evidence refs of the decision's facts_used.
func factEvidenceRefs(decisionDocument map[string]any) map[string]bool {
	refs := make(map[string]bool)
	facts, _ := decisionDocument["facts_used"].([]any)
	for _, fact := range facts {
		object, _ := fact.(map[string]any)
		if ref, ok := object["evidence"].(string); ok {
			refs[ref] = true
		}
	}
	return refs
}
