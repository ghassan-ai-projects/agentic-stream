package episodes_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// P4/T5 cross-repo conformance: the aquaculture intent catalog compiled on
// the Go side must digest to the SAME shared-domain value the Ruby worker's
// IntentCatalog computes over the identical document (fixture data authored
// once in test/support/aquaculture_domain.rb). A drift on either side breaks
// every Go-driven episode at the worker's verify_wire gate.
func TestAquacultureIntentCatalogDigestParity(t *testing.T) {
	const pinnedDigest = "sha256:7d923ca199a0878bc6d2548ef6ec64de9fd6017ae95492748a082aad3927c9cf"

	actionSchema := func() map[string]any {
		return map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"entity_id":         map[string]any{"type": "string"},
				"situation_id":      map[string]any{"type": "string"},
				"situation_version": map[string]any{"type": "integer"},
				"hypothesis":        map[string]any{"type": "string", "maxLength": 512},
			},
		}
	}
	watchSchema := func() map[string]any {
		return map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"entity_id":         map[string]any{"type": "string"},
				"situation_id":      map[string]any{"type": "string"},
				"situation_version": map[string]any{"type": "integer"},
				"metric":            map[string]any{"type": []any{"string", "number"}},
				"expression":        map[string]any{"type": []any{"string", "number"}},
				"target":            map[string]any{"type": "string"},
				"expires_at":        map[string]any{"type": "string"},
				"threshold":         map[string]any{"type": "number"},
				"max_fires":         map[string]any{"type": "integer"},
			},
		}
	}
	risks := []struct {
		Type string
		Risk string
	}{
		{"install_watch_condition", "R0"}, {"create_maintenance_ticket", "R1"},
		{"schedule_maintenance", "R1"}, {"reduce_load", "R1"},
		{"downgrade_dispatch", "R1"}, {"withdraw_ticket", "R1"},
		{"start_aerator", "R1"}, {"halt_feeding", "R1"},
		{"downgrade_intervention", "R1"}, {"withdraw_intervention", "R1"},
		{"run_vent_cycle", "R1"}, {"dehumidify", "R1"},
		{"downgrade_climate_action", "R1"}, {"withdraw_climate_action", "R1"},
		{"recommend_operating_limit", "R2"}, {"dispatch_crew", "R2"},
		{"emergency_water_exchange", "R2"}, {"deploy_shade_or_heat", "R2"},
		{"dose_co2", "R2"}, {"isolate_segment", "R3"},
	}
	intents := make([]spec.Intent, 0, len(risks))
	for _, entry := range risks {
		intent := spec.Intent{
			Type:             entry.Type,
			Risk:             entry.Risk,
			Description:      entry.Type + " (" + entry.Risk + ")",
			Policy:           "automatic",
			RateLimitPerHour: 60,
			ParameterSchema:  actionSchema(),
			ModelWritableFields: []string{"hypothesis"},
		}
		if entry.Type == "install_watch_condition" {
			intent.ParameterSchema = watchSchema()
			intent.ModelWritableFields = []string{}
			intent.Presets = map[string]map[string]any{
				"default": {
					"expression": "situation.condition_score >= 0.8",
					"metric":     "condition_score",
					"threshold":  0.8,
					"max_fires":  3,
				},
			}
		} else {
			intent.Presets = map[string]map[string]any{"default": {}}
		}
		intents = append(intents, intent)
	}

	_, digest, err := episodes.CompileIntentCatalog(intents)
	if err != nil {
		t.Fatalf("compile aquaculture catalog: %v", err)
	}
	if digest != pinnedDigest {
		t.Fatalf("intent catalog digest = %s, want the Ruby-pinned %s", digest, pinnedDigest)
	}
}
