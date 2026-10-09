package domain

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestAquacultureIntentCatalogDigestParity(t *testing.T) {
	t.Parallel()
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

	_, digest, err := CompileIntentCatalog(intents)
	if err != nil {
		t.Fatalf("compile aquaculture catalog: %v", err)
	}
	if digest != pinnedDigest {
		t.Fatalf("intent catalog digest = %s, want the Ruby-pinned %s", digest, pinnedDigest)
	}
}

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
	raw, err := os.ReadFile("../../testdata/aquaculture_intents.json")
	if err != nil {
		t.Fatalf("read aquaculture intents data: %v", err)
	}
	var document aquacultureIntentDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse aquaculture intents data: %v", err)
	}
	return &document
}

const thermalIntentCatalogDigest = "sha256:729227ecb483d69d2060e89a170f0aa30105222760ddc018ea16f3f588a4df28"

func TestThermalIntentCatalogDigestParity(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"../../../../examples/real-world-sensor/zone-thermal-sim.situation.yaml",
		"../../../../examples/real-world-sensor/zone-thermal-bench.situation.yaml",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			compiled, err := spec.CompileFile(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			_, digest, err := CompileIntentCatalog(compiled.Actions.Intents)
			if err != nil {
				t.Fatal(err)
			}
			if digest != thermalIntentCatalogDigest {
				t.Fatalf("intent catalog digest = %s, want the pinned %s; change Tamoz's thermal domain in the same change, then this pin", digest, thermalIntentCatalogDigest)
			}
		})
	}
}
