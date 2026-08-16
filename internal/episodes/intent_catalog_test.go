package episodes_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// P4/T5 cross-repo conformance: the aquaculture intent catalog compiled on
// the Go side must digest to the SAME shared-domain value the Ruby worker's
// IntentCatalog computes over the identical document (fixture data authored
// once in test/fixtures/domains/aquaculture.json, loaded via
// test/support/domain_loader.rb). A drift on either side breaks
// every Go-driven episode at the worker's verify_wire gate.
func TestAquacultureIntentCatalogDigestParity(t *testing.T) {
	const pinnedDigest = "sha256:e4f866204344a5f19994e28afdea67b610e34405b062bbfd2879209a363d81f3"

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
		Type         string
		Risk         string
		Compensation map[string]any
	}{
		{"install_watch_condition", "R0", nil},
		{"create_maintenance_ticket", "R1", map[string]any{"withdraw": "withdraw_ticket", "downgrade": "downgrade_dispatch"}},
		{"schedule_maintenance", "R1", map[string]any{"withdraw": "withdraw_ticket", "downgrade": "downgrade_dispatch"}},
		{"reduce_load", "R1", nil},
		{"downgrade_dispatch", "R1", nil},
		{"withdraw_ticket", "R1", nil},
		{"start_aerator", "R1", map[string]any{"withdraw": "withdraw_intervention", "downgrade": "downgrade_intervention"}},
		{"halt_feeding", "R1", map[string]any{"withdraw": "withdraw_intervention", "downgrade": "downgrade_intervention"}},
		{"downgrade_intervention", "R1", nil},
		{"withdraw_intervention", "R1", nil},
		{"run_vent_cycle", "R1", map[string]any{"withdraw": "withdraw_climate_action", "downgrade": "downgrade_climate_action"}},
		{"dehumidify", "R1", map[string]any{"withdraw": "withdraw_climate_action", "downgrade": "downgrade_climate_action"}},
		{"downgrade_climate_action", "R1", nil},
		{"withdraw_climate_action", "R1", nil},
		{"recommend_operating_limit", "R2", nil},
		{"dispatch_crew", "R2", map[string]any{"withdraw": "withdraw_ticket", "downgrade": "downgrade_dispatch"}},
		{"emergency_water_exchange", "R2", map[string]any{"withdraw": "withdraw_intervention", "downgrade": "downgrade_intervention"}},
		{"deploy_shade_or_heat", "R2", map[string]any{"withdraw": "withdraw_climate_action", "downgrade": "downgrade_climate_action"}},
		{"dose_co2", "R2", map[string]any{"withdraw": "withdraw_climate_action", "downgrade": "downgrade_climate_action"}},
		{"isolate_segment", "R3", nil},
		{"cancel_product_transfer", "R3", map[string]any{"withdraw": "cancel_product_transfer", "downgrade": "downgrade_product_transfer"}},
		{"downgrade_product_transfer", "R3", map[string]any{"withdraw": "cancel_product_transfer", "downgrade": "downgrade_product_transfer"}},
	}
	targetTypes := map[string]bool{}
	for _, entry := range risks {
		if entry.Compensation != nil {
			for _, target := range entry.Compensation {
				targetTypes[target.(string)] = true
			}
		}
	}
	intents := make([]spec.Intent, 0, len(risks))
	for _, entry := range risks {
		schema := actionSchema()
		if targetTypes[entry.Type] {
			// The compensate node's note/priority parameters are declared on
			// the TARGET schemas (the Ruby fixture mirrors this).
			schema = compensationSchema()
		}
		intent := spec.Intent{
			Type:             entry.Type,
			Risk:             entry.Risk,
			Description:      entry.Type + " (" + entry.Risk + ")",
			Policy:           "automatic",
			RateLimitPerHour: 60,
			ParameterSchema:  schema,
			ModelWritableFields: []string{"hypothesis"},
			Compensation:        entry.Compensation,
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

func compensationSchema() map[string]any {
	properties := map[string]any{
		"entity_id":         map[string]any{"type": "string"},
		"situation_id":      map[string]any{"type": "string"},
		"situation_version": map[string]any{"type": "integer"},
		"hypothesis":        map[string]any{"type": "string", "maxLength": 512},
		"note":              map[string]any{"type": "string", "maxLength": 512},
		"priority":          map[string]any{"type": "string", "maxLength": 16},
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
}
