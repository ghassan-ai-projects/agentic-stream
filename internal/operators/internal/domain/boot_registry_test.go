package domain_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	predictiveMaintenanceSpec = "../../../../examples/predictive-maintenance/predictive-maintenance.situation.yaml"
	customVibrationType       = "motor.vibration.custom"
	customVibrationFields     = `"rms_mm_s": {"path": "rms_mm_s", "unit": "mm_s"}, "quality": {"path": "quality", "type": "string", "enum": ["valid", "invalid"]}`
	customBootIDField         = `, "boot_id": {"path": "boot_id", "type": "string", "optional": true}`
)

func TestADirectorySchemaWithQualityAndBootIDRequiresAnIdentifiedBoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		fields      string
		wantFeature bool
	}{
		{"a schema declaring quality and boot_id requires a boot id", customVibrationFields + customBootIDField, false},
		{"a schema declaring quality alone does not", customVibrationFields, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, compiledAgainstDirectorySchema(t, tt.fields))
			withoutBoot := envelope("vibration-1", customVibrationType, base, map[string]any{"rms_mm_s": 3.0, "quality": "valid"})
			if got := hasFeature(h.applyOnTime(withoutBoot), "vibration_rms"); got != tt.wantFeature {
				t.Fatalf("vibration_rms feature from an event without a boot id = %v, want %v", got, tt.wantFeature)
			}
		})
	}
}

func TestADirectorySchemaAdmitsAnIdentifiedBoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, compiledAgainstDirectorySchema(t, customVibrationFields+customBootIDField))
	identified := envelope("vibration-1", customVibrationType, base, map[string]any{"rms_mm_s": 3.0, "quality": "valid", "boot_id": "boot-A"})
	if !hasFeature(h.applyOnTime(identified), "vibration_rms") {
		t.Fatal("an identified boot produced no vibration_rms feature")
	}
}

func compiledAgainstDirectorySchema(t *testing.T, fields string) *spec.CompiledSpec {
	t.Helper()
	source, err := os.ReadFile(predictiveMaintenanceSpec)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	specPath := filepath.Join(dir, "custom.situation.yaml")
	writeFile(t, dir, "custom.situation.yaml", strings.ReplaceAll(string(source), "motor.vibration.observed", customVibrationType))
	schemaDir := filepath.Join(dir, "schemas")
	if err := os.Mkdir(schemaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, schemaDir, "vibration.json", `{"`+customVibrationType+`/1.0": {"event_type": "`+customVibrationType+`", "schema_version": "1.0", "fields": {`+fields+`}}}`)
	schemas, err := spec.LoadEventSchemas(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), specPath, spec.WithEventSchemas(schemas))
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasFeature(features []domain.Feature, operatorID string) bool {
	for _, feature := range features {
		if feature.OperatorID == operatorID {
			return true
		}
	}
	return false
}
