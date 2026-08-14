package eventschema

import (
	"encoding/json"
	"testing"
)

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
