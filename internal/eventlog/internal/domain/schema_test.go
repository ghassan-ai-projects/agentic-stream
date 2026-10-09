package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func thermalSchema(t *testing.T) EventSchema {
	t.Helper()
	schema, err := DecodeEventSchema([]byte(`{
		"properties": {
			"celsius": {"type": "number"},
			"quality": {"type": "string", "enum": ["valid", "drift"]}
		},
		"additionalProperties": false,
		"required": ["celsius"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestCheckPayloadAcceptsDeclaredFields(t *testing.T) {
	t.Parallel()
	if err := thermalSchema(t).CheckPayload(map[string]any{"celsius": 42.5, "quality": "valid"}); err != nil {
		t.Fatalf("declared payload rejected: %v", err)
	}
}

func TestCheckPayloadRejectsViolationsWithOriginalTexts(t *testing.T) {
	t.Parallel()
	schema := thermalSchema(t)
	for name, tc := range map[string]struct {
		data map[string]any
		want string
	}{
		"undeclared field": {map[string]any{"celsius": 1, "extra": true}, `payload field "extra" is not declared`},
		"wrong type":       {map[string]any{"celsius": "hot"}, `payload field "celsius": expected number, got string`},
		"enum violation":   {map[string]any{"celsius": 1, "quality": "broken"}, `expected one of [valid drift], got "broken"`},
		"missing required": {map[string]any{"quality": "valid"}, `payload field "celsius" is required`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := schema.CheckPayload(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v want %q", err, tc.want)
			}
		})
	}
}

func TestCheckPayloadAllowsAdditionalWhenDeclared(t *testing.T) {
	t.Parallel()
	schema, err := DecodeEventSchema([]byte(`{"properties": {"a": {"type": "integer"}}, "additionalProperties": true}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.CheckPayload(map[string]any{"a": 3.0, "extra": "ok"}); err != nil {
		t.Fatalf("additional property rejected: %v", err)
	}
}

func TestDecodeEventSchemaRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	if _, err := DecodeEventSchema([]byte(`not-json`)); err == nil || !strings.Contains(err.Error(), "decode event schema") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckPayloadMatchesEachDeclaredJSONType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		declared   string
		value      any
		acceptable bool
	}{
		{"number accepts float", "number", 1.5, true},
		{"number accepts int", "number", 3, true},
		{"number accepts json.Number", "number", json.Number("2.5"), true},
		{"number refuses string", "number", "1", false},
		{"integer accepts int", "integer", 3, true},
		{"integer accepts whole float", "integer", 3.0, true},
		{"integer refuses fraction", "integer", 3.5, false},
		{"integer refuses string", "integer", "3", false},
		{"string accepts string", "string", "x", true},
		{"string refuses number", "string", 1.0, false},
		{"boolean accepts bool", "boolean", true, true},
		{"boolean refuses string", "boolean", "true", false},
		{"object accepts map", "object", map[string]any{"a": 1}, true},
		{"object refuses list", "object", []any{1}, false},
		{"array accepts list", "array", []any{1}, true},
		{"array accepts string list", "array", []string{"a"}, true},
		{"array refuses map", "array", map[string]any{}, false},
		{"null accepts nil", "null", nil, true},
		{"null refuses a value", "null", "x", false},
		{"undeclared type accepts anything", "", map[string]any{"a": 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := EventSchema{Properties: map[string]SchemaProperty{"field": {Type: tc.declared}}}
			err := schema.CheckPayload(map[string]any{"field": tc.value})
			if tc.acceptable && err != nil {
				t.Fatalf("value %#v refused for type %q: %v", tc.value, tc.declared, err)
			}
			wantRefusal := "expected " + tc.declared
			if tc.declared == "null" {
				wantRefusal = "unsupported JSON type"
			}
			if !tc.acceptable && (err == nil || !strings.Contains(err.Error(), wantRefusal)) {
				t.Fatalf("value %#v accepted for type %q, err = %v", tc.value, tc.declared, err)
			}
		})
	}
}

func TestEnumRefusesNonStringValuesAndNamesTheAllowedOnes(t *testing.T) {
	t.Parallel()
	schema := EventSchema{Properties: map[string]SchemaProperty{"quality": {Enum: []string{"valid", "drift"}}}}
	err := schema.CheckPayload(map[string]any{"quality": 3.0})
	if err == nil || !strings.Contains(err.Error(), "expected one of [valid drift], got float64") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnsupportedJSONTypeIsRejected(t *testing.T) {
	t.Parallel()
	schema, err := DecodeEventSchema([]byte(`{"properties": {"a": {"type": "unicorn"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	err = schema.CheckPayload(map[string]any{"a": 1})
	if err == nil || !strings.Contains(err.Error(), "unsupported JSON type") {
		t.Fatalf("err = %v", err)
	}
}
