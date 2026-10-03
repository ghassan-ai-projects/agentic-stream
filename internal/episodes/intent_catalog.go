package episodes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// P4 (PHASE_P4_INTENT_AUTHORITY): the intent catalog — the domain's action
// vocabulary compiled from the spec into the canonical wire form the worker
// verifies (Ruby IntentCatalog) and the validator enforces independently
// (B9/B10). The digest binds the parsed array under the shared
// situation-runtime/intent-catalog domain; a forged or malformed catalog
// fails closed on both sides.

var intentRiskRanks = map[string]int{
	"R0": 0, "R1": 1, "R2": 2, "R3": 3, "R4": 4,
}

// CompileIntentCatalog converts the spec's declared intents into the canonical
// wire catalog (the exact shape the Ruby worker parses) and digests it under
// the shared domain. A missing, empty, duplicate, or structurally invalid
// catalog is a compile error — never silently repaired.
func CompileIntentCatalog(intents []spec.Intent) ([]map[string]any, string, error) {
	if len(intents) == 0 {
		return nil, "", fmt.Errorf("intent catalog is empty")
	}
	entries, err := compileIntentEntries(intents)
	if err != nil {
		return nil, "", err
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainIntentCatalog, entries)
	if err != nil {
		return nil, "", fmt.Errorf("digest intent catalog: %w", err)
	}
	return entries, digest, nil
}

func compileIntentEntries(intents []spec.Intent) ([]map[string]any, error) {
	entries := make([]map[string]any, 0, len(intents))
	seen := make(map[string]struct{}, len(intents))
	for _, intent := range intents {
		if err := admitIntentType(intent.Type, seen); err != nil {
			return nil, err
		}
		entry, err := compileIntentEntry(intent)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func admitIntentType(intentType string, seen map[string]struct{}) error {
	if intentType == "" {
		return fmt.Errorf("intent catalog entry has no type")
	}
	if _, exists := seen[intentType]; exists {
		return fmt.Errorf("intent catalog duplicates type %q", intentType)
	}
	seen[intentType] = struct{}{}
	return nil
}

// compileIntentEntry validates one declared intent and converts it to its
// wire catalog entry.
func compileIntentEntry(intent spec.Intent) (map[string]any, error) {
	if err := validateIntentSchema(intent); err != nil {
		return nil, err
	}
	// The FULL schema is carried and digested — required/enum/minLength
	// etc. stay part of the authority, never narrowed.
	canonicalSchema, err := canonicaljson.Marshal(intent.ParameterSchema)
	if err != nil {
		return nil, fmt.Errorf("canonicalize intent %q schema: %w", intent.Type, err)
	}
	if err := validateIntentParameters(intent); err != nil {
		return nil, err
	}
	return intentCatalogEntry(intent, canonicalSchema), nil
}

// validateIntentSchema requires a declared risk and a closed object schema.
func validateIntentSchema(intent spec.Intent) error {
	if _, ok := intentRiskRanks[intent.Risk]; !ok {
		return fmt.Errorf("intent %q has invalid declared risk %q", intent.Type, intent.Risk)
	}
	if intent.ParameterSchema == nil {
		return fmt.Errorf("intent %q has no parameter schema", intent.Type)
	}
	if schemaType, _ := intent.ParameterSchema["type"].(string); schemaType != "object" {
		return fmt.Errorf("intent %q parameter schema is not an object", intent.Type)
	}
	if additional, present := intent.ParameterSchema["additionalProperties"]; !present || additional != false {
		return fmt.Errorf("intent %q parameter schema must reject unknown properties", intent.Type)
	}
	return nil
}

func validateIntentParameters(intent spec.Intent) error {
	properties, _ := intent.ParameterSchema["properties"].(map[string]any)
	for _, field := range intent.ModelWritableFields {
		if _, ok := properties[field]; !ok {
			return fmt.Errorf("intent %q marks %q writable but the schema has no such property",
				intent.Type, field)
		}
	}
	if err := validatePresets(intent.Type, properties, intent.Presets); err != nil {
		return err
	}
	if intent.RateLimitPerHour < 0 {
		return fmt.Errorf("intent %q has an invalid rate limit", intent.Type)
	}
	return nil
}

// validatePresets mirrors the Ruby IntentCatalog's preset rules: every preset
// parameter must be declared by the schema, and presets must be bounded.
func validatePresets(intentType string, properties map[string]any, presets map[string]map[string]any) error {
	if len(presets) > 64 {
		return fmt.Errorf("intent %q has too many presets", intentType)
	}
	for name, parameters := range presets {
		if len(parameters) > 128 {
			return fmt.Errorf("intent %q preset %q is too large", intentType, name)
		}
		for key := range parameters {
			if _, declared := properties[key]; !declared {
				return fmt.Errorf("intent %q preset %q references undeclared parameter %q",
					intentType, name, key)
			}
		}
	}
	return nil
}

func intentCatalogEntry(intent spec.Intent, canonicalSchema []byte) map[string]any {

	writable := intent.ModelWritableFields
	if writable == nil {
		writable = []string{}
	}
	entry := map[string]any{
		"type":                    intent.Type,
		"risk_class":              intent.Risk,
		"parameter_schema":        intent.ParameterSchema,
		"parameter_schema_digest": "sha256:" + hex.EncodeToString(sha256Sum(canonicalSchema)),
		"model_writable_fields":   writable,
	}
	addOptionalIntentFields(entry, intent)
	return entry
}

func sha256Sum(bytes []byte) []byte {
	sum := sha256.Sum256(bytes)
	return sum[:]
}

// addOptionalIntentFields adds the fields the wire form omits when unset.
func addOptionalIntentFields(entry map[string]any, intent spec.Intent) {
	if len(intent.Presets) > 0 {
		entry["presets"] = intent.Presets
	}
	if intent.Description != "" {
		entry["description"] = intent.Description
	}
	if intent.Policy != "" {
		entry["policy"] = map[string]any{"requires_approval": intent.Policy == "approval"}
	}
	if intent.RateLimitPerHour > 0 {
		entry["rate_limit"] = map[string]any{"per_hour": intent.RateLimitPerHour}
	}
	if len(intent.Compensation) > 0 {
		entry["compensation"] = intent.Compensation
	}
}
