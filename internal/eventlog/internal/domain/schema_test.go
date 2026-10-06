package domain

import (
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
