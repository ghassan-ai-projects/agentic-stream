package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestEventSchemaJSONEmitsEnumsAndRequiredFields(t *testing.T) {
	t.Parallel()

	definition := EventSchema{Fields: map[string]EventField{
		"quality": {Path: "quality", Type: "string", Optional: true, Enum: []string{"valid", "invalid"}},
		"value":   {Path: "value", Unit: "celsius"},
	}}

	raw, err := EventSchemaJSON(definition)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var document struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if !reflect.DeepEqual(document.Required, []string{"value"}) {
		t.Fatalf("required = %v, want only the non-optional value", document.Required)
	}
	if document.Properties["value"].Type != "number" {
		t.Fatalf("untyped value property = %q, want number", document.Properties["value"].Type)
	}
	quality := document.Properties["quality"]
	if quality.Type != "string" {
		t.Fatalf("quality type = %q, want string", quality.Type)
	}
	if !reflect.DeepEqual(quality.Enum, definition.Fields["quality"].Enum) {
		t.Fatalf("quality enum = %v, want %v", quality.Enum, definition.Fields["quality"].Enum)
	}
	if document.Properties["value"].Enum != nil {
		t.Fatal("value unexpectedly has an enum")
	}
}

func TestThermalSchemasDeclareQualityAndProvenance(t *testing.T) {
	t.Parallel()

	const thermalQuality = "valid|warming|invalid|disconnected|rail_high|rail_low"
	wantQuality := []string{"valid", "warming", "invalid", "disconnected", "rail_high", "rail_low"}
	common := map[string]string{
		"calibration_id": "string",
		"firmware_id":    "string",
		"schema_version": "string",
		"raw_value":      "number",
		"boot_id":        "string",
		"seq":            "integer",
		"device_mono_us": "integer",
	}
	refs := []string{
		"zone.humidity.observed/1.0",
		"zone.temp.observed/1.0",
		"zone.ambient.observed/1.0",
		"zone.fan_tach.observed/1.0",
		"zone.heartbeat.observed/1.0",
	}

	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			definition, ok := LookupEventSchema(ref)
			if !ok {
				t.Fatalf("schema %q is not registered", ref)
			}
			quality, ok := definition.Fields["quality"]
			if !ok {
				t.Fatal("quality field is not declared")
			}
			if quality.Type != "string" || !quality.Optional || !reflect.DeepEqual(quality.Enum, wantQuality) {
				t.Fatalf("quality = %+v, want optional string enum %s", quality, thermalQuality)
			}
			for name, wantType := range common {
				field, ok := definition.Fields[name]
				if !ok {
					t.Errorf("provenance field %q is not declared", name)
					continue
				}
				if field.Type != wantType || !field.Optional {
					t.Errorf("provenance field %q = %+v, want optional %s", name, field, wantType)
				}
			}

			raw, err := EventSchemaJSON(definition)
			if err != nil {
				t.Fatalf("EventSchemaJSON(%q): %v", ref, err)
			}
			var document struct {
				Properties map[string]struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatalf("decode EventSchemaJSON(%q): %v", ref, err)
			}
			if !reflect.DeepEqual(document.Properties["quality"].Enum, wantQuality) {
				t.Fatalf("generated quality enum = %v, want %v", document.Properties["quality"].Enum, wantQuality)
			}
		})
	}
}

func TestAllBuiltinsLoadFromData(t *testing.T) {
	t.Parallel()
	registry, err := builtins()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(registry) != 52 {
		t.Fatalf("expected 52 built-in refs, got %d", len(registry))
	}
	canonical, err := canonicaljson.Marshal(registry)
	if err != nil {
		t.Fatalf("canonicalize registry: %v", err)
	}
	sum := sha256.Sum256(canonical)
	const pinnedDigest = "496c1f7b52d5723be41be07d5735a2c1c9ae0b17849266ae36c8673c128ac752"
	if got := hex.EncodeToString(sum[:]); got != pinnedDigest {
		t.Fatalf("registry data digest = %s, want the pinned %s", got, pinnedDigest)
	}
	for ref, definition := range registry {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			if definition.Ref != ref {
				t.Fatalf("Ref = %q, want %q", definition.Ref, ref)
			}
			if definition.EventType == "" {
				t.Fatal("event type must not be empty")
			}
			if definition.SchemaVersion == "" {
				t.Fatal("schema version must not be empty")
			}
			for name, field := range definition.Fields {
				if field.Path == "" {
					t.Fatalf("field %s has no path", name)
				}
				switch {
				case field.Enum != nil:
					if field.Type != "string" || len(field.Enum) == 0 {
						t.Fatalf("enum field %s = %+v, want non-empty string enum", name, field)
					}
				case name == "unit":
					if field.Type != "string" || !field.Optional {
						t.Fatalf("unit field = %+v, want string-typed optional", field)
					}
				case field.Unit != "":
					if field.Type != "" && field.Type != "number" {
						t.Fatalf("numeric field %s = %+v, type must be empty or number", name, field)
					}
				default:
					if field.Type == "" {
						t.Fatalf("field %s is neither typed nor numeric-with-unit: %+v", name, field)
					}
				}
			}
		})
	}
}
