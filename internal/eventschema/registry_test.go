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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
