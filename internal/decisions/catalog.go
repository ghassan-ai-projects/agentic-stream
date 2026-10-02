package decisions

import (
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// IntentCatalog is the compiled catalog the validator enforces independently.
type IntentCatalog struct {
	Entries map[string]*IntentEntry
}

// IntentEntry is one declared action type's authority.
type IntentEntry struct {
	Type             string
	RiskClass        string
	ParameterSchema  *jsonschema.Schema
	Presets          map[string]map[string]any
	ModelWritable    map[string]bool
	RateLimitPerHour int
	RequiresApproval bool
}

// CompileIntentCatalog builds the fail-closed validator view from the wire
// document (already digest-verified by the caller): a missing, empty,
// duplicate, or structurally invalid catalog is an error — never repaired.
func CompileIntentCatalog(doc []map[string]any) (*IntentCatalog, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("intent catalog is empty")
	}
	catalog := &IntentCatalog{Entries: make(map[string]*IntentEntry, len(doc))}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	for _, entry := range doc {
		entryType, _ := entry["type"].(string)
		if entryType == "" {
			return nil, fmt.Errorf("intent catalog entry has no type")
		}
		if _, exists := catalog.Entries[entryType]; exists {
			return nil, fmt.Errorf("intent catalog duplicates type %q", entryType)
		}
		compiled, err := compileCatalogEntry(compiler, entryType, entry)
		if err != nil {
			return nil, err
		}
		catalog.Entries[entryType] = compiled
	}
	return catalog, nil
}

// compileCatalogEntry validates one wire catalog entry and compiles its
// parameter schema.
func compileCatalogEntry(compiler *jsonschema.Compiler, entryType string, entry map[string]any) (*IntentEntry, error) {
	risk, _ := entry["risk_class"].(string)
	if riskRank(risk) == 0 {
		return nil, fmt.Errorf("intent %q has invalid declared risk %q", entryType, risk)
	}
	schema, ok := entry["parameter_schema"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("intent %q has no parameter schema", entryType)
	}
	schemaID := "urn:situation-runtime:catalog:" + entryType + ":schema:v1"
	if err := compiler.AddResource(schemaID, schema); err != nil {
		return nil, fmt.Errorf("compile intent %q schema: %w", entryType, err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("compile intent %q schema: %w", entryType, err)
	}
	return &IntentEntry{
		Type:             entryType,
		RiskClass:        risk,
		ParameterSchema:  compiled,
		Presets:          catalogPresets(entry),
		ModelWritable:    stringSet(toStringSlice(entry["model_writable_fields"])),
		RateLimitPerHour: intValue(entry["rate_limit"], "per_hour"),
		RequiresApproval: requiresApproval(entry),
	}, nil
}

func catalogPresets(entry map[string]any) map[string]map[string]any {
	presets := make(map[string]map[string]any)
	raw, _ := entry["presets"].(map[string]any)
	for name, value := range raw {
		if parameters, ok := value.(map[string]any); ok {
			presets[name] = parameters
		}
	}
	return presets
}

// requiresApproval reads the catalog policy, which is part of the
// digest-bound authority: a declared "requires_approval" intent must never
// auto-dispatch.
func requiresApproval(entry map[string]any) bool {
	policy, _ := entry["policy"].(map[string]any)
	flag, _ := policy["requires_approval"].(bool)
	return flag
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func toStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func intValue(section any, key string) int {
	raw, ok := section.(map[string]any)
	if !ok {
		return 0
	}
	if value, ok := raw[key].(float64); ok {
		return int(value)
	}
	return 0
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}
