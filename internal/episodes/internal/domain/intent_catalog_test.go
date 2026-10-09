package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func validIntent() spec.Intent {
	return spec.Intent{Type: "create_ticket", Risk: "R1", ParameterSchema: catalogTicketSchema()}
}

func TestCompileIntentCatalogRejectsAnInvalidCatalog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		intents func() []spec.Intent
		want    string
	}{
		{"empty", func() []spec.Intent { return nil }, "intent catalog is empty"},
		{"no type", func() []spec.Intent { i := validIntent(); i.Type = ""; return []spec.Intent{i} }, "has no type"},
		{"duplicate type", func() []spec.Intent { return []spec.Intent{validIntent(), validIntent()} }, `duplicates type "create_ticket"`},
		{"undeclared risk", func() []spec.Intent { i := validIntent(); i.Risk = ""; return []spec.Intent{i} }, "invalid declared risk"},
		{"unknown risk", func() []spec.Intent { i := validIntent(); i.Risk = "R9"; return []spec.Intent{i} }, "invalid declared risk"},
		{"no schema", func() []spec.Intent { i := validIntent(); i.ParameterSchema = nil; return []spec.Intent{i} }, "has no parameter schema"},
		{"schema not an object", func() []spec.Intent {
			i := validIntent()
			i.ParameterSchema = map[string]any{"type": "string", "additionalProperties": false}
			return []spec.Intent{i}
		}, "parameter schema is not an object"},
		{"schema open to unknown properties", func() []spec.Intent {
			i := validIntent()
			i.ParameterSchema = map[string]any{"type": "object"}
			return []spec.Intent{i}
		}, "must reject unknown properties"},
		{"schema allows unknown properties", func() []spec.Intent {
			i := validIntent()
			i.ParameterSchema = map[string]any{"type": "object", "additionalProperties": true}
			return []spec.Intent{i}
		}, "must reject unknown properties"},
		{"writable field outside the schema", func() []spec.Intent {
			i := validIntent()
			i.ModelWritableFields = []string{"severity"}
			return []spec.Intent{i}
		}, `marks "severity" writable`},
		{"preset parameter outside the schema", func() []spec.Intent {
			i := validIntent()
			i.Presets = map[string]map[string]any{"default": {"severity": "high"}}
			return []spec.Intent{i}
		}, `preset "default" references undeclared parameter "severity"`},
		{"too many presets", func() []spec.Intent {
			i := validIntent()
			i.Presets = map[string]map[string]any{}
			for n := range 65 {
				i.Presets[strings.Repeat("p", n+1)] = map[string]any{}
			}
			return []spec.Intent{i}
		}, "too many presets"},
		{"oversized preset", func() []spec.Intent {
			i := validIntent()
			large := map[string]any{}
			for n := range 129 {
				large[strings.Repeat("k", n+1)] = 1
			}
			i.Presets = map[string]map[string]any{"big": large}
			return []spec.Intent{i}
		}, `preset "big" is too large`},
		{"negative rate limit", func() []spec.Intent { i := validIntent(); i.RateLimitPerHour = -1; return []spec.Intent{i} }, "invalid rate limit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog, digest, err := CompileIntentCatalog(tc.intents())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
			if catalog != nil || digest != "" {
				t.Fatalf("rejected catalog leaked entries=%v digest=%q", catalog, digest)
			}
		})
	}
}

func TestCompileIntentCatalogCarriesOnlyTheDeclaredOptionalFields(t *testing.T) {
	t.Parallel()
	bare, _, err := CompileIntentCatalog([]spec.Intent{validIntent()})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"presets", "description", "policy", "rate_limit", "compensation"} {
		if _, present := bare[0][field]; present {
			t.Errorf("bare intent carries %s", field)
		}
	}
	if writable, _ := bare[0]["model_writable_fields"].([]string); writable == nil || len(writable) != 0 {
		t.Fatalf("model_writable_fields = %#v, want an empty list", bare[0]["model_writable_fields"])
	}

	full := validIntent()
	full.Description = "open a ticket"
	full.Policy = "approval"
	full.RateLimitPerHour = 4
	full.ModelWritableFields = []string{"reason"}
	full.Presets = map[string]map[string]any{"default": {"reason": "x"}}
	full.Compensation = map[string]any{"undo": "withdraw_ticket"}
	entries, _, err := CompileIntentCatalog([]spec.Intent{full})
	if err != nil {
		t.Fatal(err)
	}
	entry := entries[0]
	if entry["description"] != "open a ticket" || entry["policy"].(map[string]any)["requires_approval"] != true ||
		entry["rate_limit"].(map[string]any)["per_hour"] != 4 || entry["compensation"] == nil || entry["presets"] == nil {
		t.Fatalf("entry = %#v", entry)
	}
	automatic := validIntent()
	automatic.Policy = "automatic"
	entries, _, err = CompileIntentCatalog([]spec.Intent{automatic})
	if err != nil {
		t.Fatal(err)
	}
	if entries[0]["policy"].(map[string]any)["requires_approval"] != false {
		t.Fatalf("automatic policy = %#v", entries[0]["policy"])
	}
}

func TestCompileIntentCatalogDigestBindsTheCatalogOrder(t *testing.T) {
	t.Parallel()
	other := validIntent()
	other.Type = "withdraw_ticket"
	_, forward, err := CompileIntentCatalog([]spec.Intent{validIntent(), other})
	if err != nil {
		t.Fatal(err)
	}
	_, again, _ := CompileIntentCatalog([]spec.Intent{validIntent(), other})
	_, reversed, _ := CompileIntentCatalog([]spec.Intent{other, validIntent()})
	if forward != again || forward == reversed {
		t.Fatalf("forward=%s again=%s reversed=%s, want a stable digest that depends on order", forward, again, reversed)
	}
}
