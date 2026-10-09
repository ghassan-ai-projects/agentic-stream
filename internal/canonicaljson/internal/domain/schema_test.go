package domain

import (
	"strings"
	"testing"
)

const testSchemaID = "urn:test:schema:v1"

func TestCompileSchemaPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schema   string
		document any
		compile  string
		validate string
	}{
		{name: "plain object", schema: `{"type":"object"}`, document: map[string]any{}},
		{name: "wrong type is rejected", schema: `{"type":"object"}`, document: "x", validate: "want object"},
		{name: "valid date-time", schema: `{"type":"string","format":"date-time"}`, document: "2026-10-08T00:00:00Z"},
		{name: "format is asserted", schema: `{"type":"string","format":"date-time"}`, document: "yesterday", validate: "date-time"},
		{name: "email format is asserted", schema: `{"type":"string","format":"email"}`, document: "nobody", validate: "email"},
		{name: "http ref is refused", schema: `{"$ref":"https://example.invalid/schema.json"}`, compile: "external schema load denied"},
		{name: "file ref is refused", schema: `{"$ref":"file:///etc/schema.json"}`, compile: "external schema load denied"},
		{name: "malformed schema", schema: `{"type":42}`, compile: "compile schema " + testSchemaID},
		{name: "not json", schema: `{`, compile: "decode schema " + testSchemaID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema, err := CompileSchemaJSON(testSchemaID, []byte(tt.schema))
			if tt.compile != "" {
				requireErrorContaining(t, err, tt.compile)
				return
			}
			if err != nil {
				t.Fatalf("CompileSchemaJSON() = %v", err)
			}
			err = schema.Validate(tt.document)
			if tt.validate == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			requireErrorContaining(t, err, tt.validate)
		})
	}
}

func TestCompileSchemaAcceptsDecodedDocument(t *testing.T) {
	t.Parallel()

	schema, err := CompileSchema(testSchemaID, map[string]any{"type": "string"})
	if err != nil {
		t.Fatalf("CompileSchema() = %v", err)
	}
	if err := schema.Validate("ok"); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want it to contain %q", err, want)
	}
}
