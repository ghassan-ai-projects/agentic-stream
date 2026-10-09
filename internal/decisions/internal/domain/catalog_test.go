package domain

import (
	"strings"
	"testing"
)

func TestCompileIntentCatalogFailsClosed(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object"}
	tests := []struct {
		name    string
		entries []map[string]any
		want    string
	}{
		{"nil catalog", nil, "intent catalog is empty"},
		{"empty catalog", []map[string]any{}, "intent catalog is empty"},
		{"entry without a type", []map[string]any{{"risk_class": "R1", "parameter_schema": schema}}, "has no type"},
		{"duplicate type", append(testCatalog(), testCatalog()[0]), `duplicates type "create_ticket"`},
		{"unknown risk class", []map[string]any{{"type": "x", "risk_class": "R9", "parameter_schema": schema}}, "invalid declared risk"},
		{"missing risk class", []map[string]any{{"type": "x", "parameter_schema": schema}}, "invalid declared risk"},
		{"missing parameter schema", []map[string]any{{"type": "x", "risk_class": "R1"}}, "has no parameter schema"},
		{"external parameter schema", []map[string]any{{"type": "x", "risk_class": "R1", "parameter_schema": map[string]any{"$ref": "https://example.invalid/schema.json"}}}, `compile intent "x" schema`},
		{"invalid parameter schema", []map[string]any{{"type": "x", "risk_class": "R1", "parameter_schema": map[string]any{"type": "no-such-type"}}}, `compile intent "x" schema`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			catalog, err := CompileIntentCatalog(test.entries)
			if err == nil || !strings.Contains(err.Error(), test.want) || catalog != nil {
				t.Fatalf("catalog = %v err = %v, want an error containing %q", catalog, err, test.want)
			}
		})
	}
}

func TestCompiledCatalogIsIndependentOfItsSource(t *testing.T) {
	t.Parallel()
	source := identityCatalog()
	source[0]["presets"] = map[string]any{"default": map[string]any{
		"priority": "routine", "labels": []any{"original"},
	}}
	catalog := compileCatalog(t, source)

	preset := source[0]["presets"].(map[string]any)["default"].(map[string]any)
	preset["priority"] = "attacker"
	preset["labels"].([]any)[0] = "attacker"
	properties := source[0]["parameter_schema"].(map[string]any)["properties"].(map[string]any)
	properties["priority"] = map[string]any{"type": "integer"}

	got := catalog.entries["act"].Presets["default"]
	if got["priority"] != "routine" || got["labels"].([]any)[0] != "original" {
		t.Fatalf("compiled preset = %v, want the values at compile time", got)
	}
	input := actInput(t)
	input.IntentCatalog = catalog
	if _, err := validateDocument(t, actDecision(map[string]any{"priority": "routine"}), input); err != nil {
		t.Fatalf("a string priority must still satisfy the compiled schema: %v", err)
	}
}
