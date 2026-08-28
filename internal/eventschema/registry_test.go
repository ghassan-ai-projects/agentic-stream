package eventschema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestJSONEmitsStringEnum(t *testing.T) {
	t.Parallel()

	definition := Definition{Fields: map[string]Field{
		"quality": {Path: "quality", Type: "string", Optional: true, Enum: []string{"valid", "invalid"}},
		"value":   {Path: "value", Unit: "celsius"},
	}}

	raw, err := JSON(definition)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var document struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode JSON: %v", err)
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
		"zone.temp.observed/1.0",
		"zone.ambient.observed/1.0",
		"zone.fan_tach.observed/1.0",
		"zone.heartbeat.observed/1.0",
	}

	for _, ref := range refs {
		ref := ref
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			definition, ok := Lookup(ref)
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

			raw, err := JSON(definition)
			if err != nil {
				t.Fatalf("JSON(%q): %v", ref, err)
			}
			var document struct {
				Properties map[string]struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatalf("decode JSON(%q): %v", ref, err)
			}
			if !reflect.DeepEqual(document.Properties["quality"].Enum, wantQuality) {
				t.Fatalf("generated quality enum = %v, want %v", document.Properties["quality"].Enum, wantQuality)
			}
		})
	}
}

func TestRotatingMachinerySchemasDescribeAdapterPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref          string
		required     []string
		optional     []string
		stringFields []string
	}{
		{ref: "pump.vibration.observed/1.0", required: []string{"rms_mm_s"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.motor_current.observed/1.0", required: []string{"value"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.discharge_pressure.observed/1.0", required: []string{"kpa"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.flow_rate.observed/1.0", required: []string{"l_s"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.tank_level.observed/1.0", required: []string{"percent"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.turbidity.observed/1.0", required: []string{"ntu"}, optional: []string{"unit"}, stringFields: []string{"unit"}},
		{ref: "pump.demand_event.observed/1.0", required: []string{"magnitude"}, stringFields: []string{}},
		{ref: "pump.mode.observed/1.0", required: []string{"mode", "value"}, stringFields: []string{"mode", "value"}},
		{ref: "pump.heartbeat.observed/1.0", required: []string{}, stringFields: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()

			definition, ok := Lookup(tt.ref)
			if !ok {
				t.Fatalf("schema %q is not registered", tt.ref)
			}
			raw, err := JSON(definition)
			if err != nil {
				t.Fatalf("JSON(%q): %v", tt.ref, err)
			}
			var document struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatalf("decode JSON(%q): %v", tt.ref, err)
			}
			for _, field := range tt.required {
				if !contains(document.Required, field) {
					t.Errorf("required fields %v do not include %q", document.Required, field)
				}
			}
			for _, field := range tt.optional {
				if contains(document.Required, field) {
					t.Errorf("optional field %q is required", field)
				}
			}
			if definition.EventType != "pump."+tt.ref[len("pump."):len(tt.ref)-len("/1.0")] {
				t.Errorf("event type %q does not match ref %q", definition.EventType, tt.ref)
			}
			for _, field := range tt.stringFields {
				if got := document.Properties[field].Type; got != "string" {
					t.Errorf("property %q type = %q, want string", field, got)
				}
			}
		})
	}
}

func TestPondDissolvedOxygenSchemaDescribesAdapterPayload(t *testing.T) {
	t.Parallel()

	ref := "pond.dissolved_oxygen.observed/1.0"
	definition, ok := Lookup(ref)
	if !ok {
		t.Fatalf("schema %q is not registered", ref)
	}
	if definition.EventType != "pond.dissolved_oxygen.observed" {
		t.Fatalf("event type = %q, want pond.dissolved_oxygen.observed", definition.EventType)
	}
	field, ok := definition.Fields["mg_l"]
	if !ok {
		t.Fatal("schema does not declare mg_l")
	}
	if field.Path != "mg_l" || field.Unit != "mg_l" {
		t.Fatalf("mg_l field = %+v, want path and unit mg_l", field)
	}

	raw, err := JSON(definition)
	if err != nil {
		t.Fatalf("JSON(%q): %v", ref, err)
	}
	var document struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode JSON(%q): %v", ref, err)
	}
	if !contains(document.Required, "mg_l") {
		t.Fatalf("required fields %v do not include mg_l", document.Required)
	}
	if contains(document.Required, "unit") || document.Properties["unit"].Type != "string" {
		t.Fatalf("unit should be an optional string field: %+v", document.Properties["unit"])
	}
}

func TestBaySchemasDescribeAdapterPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref       string
		eventType string
		field     string
		unit      string
	}{
		{ref: "bay.humidity.observed/1.0", eventType: "bay.humidity.observed", field: "percent", unit: "percent"},
		{ref: "bay.leaf_wetness.observed/1.0", eventType: "bay.leaf_wetness.observed", field: "percent", unit: "percent"},
		{ref: "bay.air_temp.observed/1.0", eventType: "bay.air_temp.observed", field: "celsius", unit: "celsius"},
		{ref: "bay.co2.observed/1.0", eventType: "bay.co2.observed", field: "umol_mol", unit: "umol_mol"},
		{ref: "bay.par_light.observed/1.0", eventType: "bay.par_light.observed", field: "value", unit: "umol_m2_s"},
		{ref: "bay.vent_position.observed/1.0", eventType: "bay.vent_position.observed", field: "percent", unit: "percent"},
		{ref: "bay.vent_event.observed/1.0", eventType: "bay.vent_event.observed", field: "magnitude", unit: "1"},
		{ref: "bay.heartbeat.observed/1.0", eventType: "bay.heartbeat.observed"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()

			definition, ok := Lookup(tt.ref)
			if !ok {
				t.Fatalf("schema %q is not registered", tt.ref)
			}
			if definition.EventType != tt.eventType {
				t.Fatalf("event type = %q, want %q", definition.EventType, tt.eventType)
			}
			if tt.field == "" {
				if len(definition.Fields) != 0 {
					t.Fatalf("fields = %+v, want empty", definition.Fields)
				}
				return
			}
			field, ok := definition.Fields[tt.field]
			if !ok {
				t.Fatalf("schema does not declare %s", tt.field)
			}
			if field.Path != tt.field || field.Unit != tt.unit {
				t.Fatalf("%s field = %+v, want path %q and unit %q", tt.field, field, tt.field, tt.unit)
			}
		})
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Domain-data extraction (docs/design/impl/GO_DOMAIN_DATA_EXTRACTION.md):
// the catalog now lives in registry_data.json. Every ref must load and expose
// its declared fields. The golden digest pins every ref's event_type /
// schema_version / fields (path, unit, type, optional, enum) VALUES — a typo in any
// entry, even one the targeted tables above do not cover, breaks the digest.
// The invariants below catch structural nonsense (a field that is neither
// typed nor numeric-with-unit) with a readable failure.
func TestAllBuiltinsLoadFromData(t *testing.T) {
	t.Parallel()
	registry, err := builtins()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(registry) != 51 {
		t.Fatalf("expected 51 built-in refs, got %d", len(registry))
	}
	canonical, err := canonicaljson.Marshal(registry)
	if err != nil {
		t.Fatalf("canonicalize registry: %v", err)
	}
	sum := sha256.Sum256(canonical)
	const pinnedDigest = "2963f017014b54673e7898ebf71a7a42d41fc906c0115296ab87be0bf62de2fe"
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
