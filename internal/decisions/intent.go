package decisions

import (
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"time"
)

func validateIntent(document map[string]any, input Input, decisionID string, decisionDocument map[string]any, seenIDs map[string]struct{}) (*Intent, error) {
	if err := checkIntentBinding(document, input, decisionID, seenIDs); err != nil {
		return nil, err
	}
	entry, err := checkIntentAuthority(document, input)
	if err != nil {
		return nil, err
	}
	// P4: parameters must satisfy the catalog's per-intent schema.
	parameters, _ := document["parameters"].(map[string]any)
	if err := entry.ParameterSchema.Validate(parameters); err != nil {
		return nil, reject("parameter_schema_violation", "intent.parameters", err.Error())
	}
	if err := checkIdentityParameters(parameters, input, decisionDocument); err != nil {
		return nil, err
	}
	// P4: preset-only fields must be byte-identical to the compiled preset —
	// an attempt to silently substitute a preset value is rejected, not
	// repaired.
	if err := verifyPresetEquality(document, entry); err != nil {
		return nil, err
	}
	// P4: the intent's evidence must be grounded in the decision's facts.
	if err := verifyEvidenceBinding(document, decisionDocument); err != nil {
		return nil, err
	}
	return buildIntent(document, input, entry)
}

// checkIntentBinding requires a unique intent bound to its Decision and to the
// episode's tenant, Situation, and version.
func checkIntentBinding(document map[string]any, input Input, decisionID string, seenIDs map[string]struct{}) error {
	intentID, _ := document["intent_id"].(string)
	if _, exists := seenIDs[intentID]; exists {
		return reject("schema_invalid", "intent_id", "Decision contains duplicate intent_id values")
	}
	seenIDs[intentID] = struct{}{}
	if got, _ := document["decision_id"].(string); got != decisionID {
		return reject("snapshot_mismatch", "intent.decision_id", "intent is not bound to its Decision")
	}
	if got, _ := document["tenant_id"].(string); got != input.TenantID {
		return reject("snapshot_mismatch", "intent.tenant_id", "intent tenant does not match the episode")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return reject("snapshot_mismatch", "intent.situation_id", "intent Situation does not match the episode")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return reject("snapshot_mismatch", "intent.situation_version", "intent Situation version does not match the episode")
	}
	return nil
}

// checkIntentAuthority checks the intent type against the episode allowlist
// and the catalog, and its risk against the catalog and the ceiling. It
// returns the catalog entry.
func checkIntentAuthority(document map[string]any, input Input) (*IntentEntry, error) {
	intentType, _ := document["type"].(string)
	isCompensation := documentString(document, "compensates") != ""
	// The allowlist bypass is kind-scoped: only a RECONSIDER episode may carry
	// compensating intents. A DIAGNOSE worker forging compensates is rejected.
	if isCompensation && input.Kind != "reconsider" {
		return nil, reject("intent_type_not_allowed", "intent.compensates",
			"compensating intents require a reconsider episode")
	}
	if !isCompensation {
		if _, allowed := input.AllowedIntentTypes[intentType]; !allowed {
			return nil, reject("intent_type_not_allowed", "intent.type", "intent type is not allowed for this episode")
		}
	}
	// P4/B10: the catalog is the authority. The type must be declared; the
	// proposed risk must EQUAL the declared risk — a risk-label attack (the
	// worker claiming R0 for an R2 action) is rejected, not merely clamped to
	// the ceiling.
	entry, declared := input.IntentCatalog.Entries[intentType]
	if !declared {
		return nil, reject("intent_type_not_in_catalog", "intent.type", "intent type is not declared in the intent catalog")
	}
	risk, _ := document["risk_class"].(string)
	if risk != entry.RiskClass {
		return nil, reject("risk_label_mismatch", "intent.risk_class",
			fmt.Sprintf("proposed risk %s does not equal the declared %s for %s", risk, entry.RiskClass, intentType))
	}
	if riskRank(risk) > riskRank(input.RiskCeiling) {
		return nil, reject("risk_ceiling_exceeded", "intent.risk_class", "intent risk exceeds the episode ceiling")
	}
	return entry, nil
}

// checkIdentityParameters binds identity-bearing parameters to the dispatched
// episode (P4): a tampered entity_id would otherwise flow into the command
// payload unverified, since the preset check only covers preset keys.
func checkIdentityParameters(parameters map[string]any, input Input, decisionDocument map[string]any) error {
	if entityID, present := parameters["entity_id"]; present {
		if input.EntityID == "" {
			return reject("snapshot_mismatch", "intent.parameters.entity_id",
				"the validator has no entity identity to bind against")
		}
		if entityID != input.EntityID {
			return reject("snapshot_mismatch", "intent.parameters.entity_id",
				"intent entity does not match the dispatched episode")
		}
	}
	if expiresAt, present := parameters["expires_at"]; present {
		if validUntil, ok := decisionDocument["valid_until"].(string); ok && expiresAt != validUntil {
			return reject("snapshot_mismatch", "intent.parameters.expires_at",
				"intent parameter expires_at must equal the decision's valid_until")
		}
	}
	// target is an identity-bearing parameter that the policy plane prefers as
	// the effector's normalized_target, so it must bind to the dispatched
	// episode's trusted identity directly — never merely to the optional
	// entity_id parameter, which may be absent. Binding to entity_id alone left
	// a hole: target without entity_id skipped verification entirely, letting a
	// proposal steer an effect at an unverified target.
	if target, present := parameters["target"]; present {
		if input.EntityID == "" {
			return reject("snapshot_mismatch", "intent.parameters.target",
				"the validator has no entity identity to bind against")
		}
		if target != input.EntityID {
			return reject("snapshot_mismatch", "intent.parameters.target",
				"intent target must equal the bound entity")
		}
	}
	return nil
}

// buildIntent checks the intent expiry and builds the validated Intent.
func buildIntent(document map[string]any, input Input, entry *IntentEntry) (*Intent, error) {
	expiresAtString, _ := document["expires_at"].(string)
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresAtString)
	if err != nil {
		return nil, reject("schema_invalid", "intent.expires_at", err.Error())
	}
	if !expiresAt.After(input.Now) {
		return nil, reject("expired", "intent.expires_at", "intent has expired")
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent", err.Error())
	}
	digest, err := contractsv1.IntentDigest(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent_digest", err.Error())
	}
	intentID, _ := document["intent_id"].(string)
	intentType, _ := document["type"].(string)
	risk, _ := document["risk_class"].(string)
	return &Intent{
		ID:               intentID,
		Type:             intentType,
		RiskClass:        risk,
		ExpiresAt:        expiresAt,
		Digest:           digest,
		CanonicalJSON:    canonical,
		Document:         document,
		RateLimitPerHour: entry.RateLimitPerHour,
		RequiresApproval: entry.RequiresApproval,
	}, nil
}

// verifyPresetEquality rejects any parameter whose key is NOT model-writable
// and IS present in the catalog's default preset but whose value differs from
// the preset's — the worker may only override the fields the catalog marks
// writable. Unknown keys (not in the preset, not writable) are schema-level
// noise and fail the schema check already.
func verifyPresetEquality(document map[string]any, entry *IntentEntry) error {
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
	grounded := make(map[string]bool)
	if facts, ok := decisionDocument["facts_used"].([]any); ok {
		for _, fact := range facts {
			if object, ok := fact.(map[string]any); ok {
				if ref, ok := object["evidence"].(string); ok {
					grounded[ref] = true
				}
			}
		}
	}
	for _, ref := range evidenceIDs {
		text, ok := ref.(string)
		if !ok {
			return reject("parameter_schema_violation", "intent.evidence_ids", "evidence id must be a string")
		}
		if !grounded[text] {
			return reject("ungrounded_evidence", "intent.evidence_ids",
				fmt.Sprintf("evidence id %s is not among the decision's facts_used", text))
		}
	}
	return nil
}
