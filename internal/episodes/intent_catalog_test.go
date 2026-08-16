package episodes_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// P4/T5 cross-repo conformance: the aquaculture intent catalog compiled on
// the Go side must digest to the SAME shared-domain value the Ruby worker's
// IntentCatalog computes over the identical document (fixture data authored
// once in test/fixtures/domains/aquaculture.json, loaded via
// test/support/domain_loader.rb). The Go copy lives as DATA in
// testdata/aquaculture_intents.json (domain-knowledge extraction —
// docs/design/impl/GO_DOMAIN_DATA_EXTRACTION.md); this test supplies only
// the derivation machinery. A drift on either side breaks every Go-driven
// episode at the worker's verify_wire gate.
func TestAquacultureIntentCatalogDigestParity(t *testing.T) {
	const pinnedDigest = "sha256:e4f866204344a5f19994e28afdea67b610e34405b062bbfd2879209a363d81f3"

	document := loadAquacultureIntents(t)

	targetTypes := map[string]bool{}
	for _, row := range document.Intents {
		for _, target := range row.Compensation {
			targetTypes[target.(string)] = true
		}
	}

	intents := make([]spec.Intent, 0, len(document.Intents))
	for _, row := range document.Intents {
		schema := document.ActionSchema
		writable := []string{"hypothesis"}
		presets := map[string]map[string]any{"default": {}}
		if targetTypes[row.Type] {
			// The compensate node's note/priority parameters are declared on
			// the TARGET schemas (the Ruby fixture mirrors this).
			schema = document.CompensationSchema
		}
		if row.Type == "install_watch_condition" {
			schema = document.WatchSchema
			writable = []string{}
			presets = map[string]map[string]any{"default": document.WatchPreset}
		}
		intents = append(intents, spec.Intent{
			Type:                row.Type,
			Risk:                row.Risk,
			Description:         row.Type + " (" + row.Risk + ")",
			Policy:              document.Policy,
			RateLimitPerHour:    document.RateLimitPerHour,
			ParameterSchema:     schema,
			ModelWritableFields: writable,
			Presets:             presets,
			Compensation:        row.Compensation,
		})
	}

	_, digest, err := episodes.CompileIntentCatalog(intents)
	if err != nil {
		t.Fatalf("compile aquaculture catalog: %v", err)
	}
	if digest != pinnedDigest {
		t.Fatalf("intent catalog digest = %s, want the Ruby-pinned %s", digest, pinnedDigest)
	}
}

// aquacultureIntentDocument mirrors testdata/aquaculture_intents.json: the
// 22 intent rows plus the schema-builder data and construction constants.
type aquacultureIntentDocument struct {
	ActionSchema       map[string]any         `json:"action_schema"`
	WatchSchema        map[string]any         `json:"watch_schema"`
	CompensationSchema map[string]any         `json:"compensation_schema"`
	WatchPreset        map[string]any         `json:"watch_preset"`
	Policy             string                 `json:"policy"`
	RateLimitPerHour   int                    `json:"rate_limit_per_hour"`
	Intents            []aquacultureIntentRow `json:"intents"`
}

type aquacultureIntentRow struct {
	Type         string         `json:"type"`
	Risk         string         `json:"risk"`
	Compensation map[string]any `json:"compensation"`
}

func loadAquacultureIntents(t *testing.T) *aquacultureIntentDocument {
	t.Helper()
	raw, err := os.ReadFile("testdata/aquaculture_intents.json")
	if err != nil {
		t.Fatalf("read aquaculture intents data: %v", err)
	}
	var document aquacultureIntentDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse aquaculture intents data: %v", err)
	}
	return &document
}
