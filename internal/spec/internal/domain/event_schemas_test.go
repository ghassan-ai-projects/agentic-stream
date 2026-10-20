package domain_test

import (
	"os"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

const fanSpeedSchema = `{"fan.speed.observed/1.0": {"event_type": "fan.speed.observed", "schema_version": "1.0",
  "fields": {"rpm": {"path": "rpm", "unit": "rpm"}}}}`

const builtinBayHeartbeat = `{"bay.heartbeat.observed/1.0": {"event_type": "bay.heartbeat.observed", "schema_version": "1.0", "fields": {}}}`

func TestMergingAddsSchemasAndKeepsTheBuiltIns(t *testing.T) {
	t.Parallel()
	merged, err := domain.BuiltinEventSchemas().Merge("fan.json", []byte(fanSpeedSchema))
	if err != nil {
		t.Fatal(err)
	}
	fan, ok := merged.Lookup("fan.speed.observed/1.0")
	if !ok || fan.EventType != "fan.speed.observed" || fan.Fields["rpm"].Unit != "rpm" {
		t.Fatalf("merged fan schema = %+v, %t", fan, ok)
	}
	if _, ok := merged.Lookup("zone.temp.observed/1.0"); !ok {
		t.Fatal("merging dropped a built-in schema")
	}
	if _, ok := domain.BuiltinEventSchemas().Lookup("fan.speed.observed/1.0"); ok {
		t.Fatal("merging changed the built-in registry")
	}
}

func TestMergingAcceptsAnIdenticalBuiltInSchema(t *testing.T) {
	t.Parallel()
	if _, err := domain.BuiltinEventSchemas().Merge("same.json", []byte(builtinBayHeartbeat)); err != nil {
		t.Fatalf("an identical built-in schema was refused: %v", err)
	}
}

func TestMergingRefusesAMalformedOrConflictingSchema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, document, want string
	}{
		{"a built-in ref with different fields", strings.Replace(builtinBayHeartbeat, `"fields": {}`, `"fields": {"beat": {"path": "beat"}}`, 1), `event schema "bay.heartbeat.observed/1.0" is already registered with a different definition`},
		{"a ref that does not name its type and version", strings.Replace(fanSpeedSchema, `"fan.speed.observed/1.0"`, `"fan/1.0"`, 1), `event schema "fan/1.0" must be named "fan.speed.observed/1.0"`},
		{"a schema without an event type", strings.Replace(fanSpeedSchema, `"event_type": "fan.speed.observed", `, "", 1), "must declare event_type and schema_version"},
		{"an unknown field", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "scale": 2`, 1), `unknown field "scale"`},
		{"trailing data", fanSpeedSchema + " {}", "trailing data after the schema document"},
		{"not JSON", "{", "decode event schemas bad.json"},
		{"a field type outside the allowed set", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "type": "banana"`, 1), `event schema "fan.speed.observed/1.0" field "rpm": type "banana" is not one of number, integer, string, boolean`},
		{"an empty path", strings.Replace(fanSpeedSchema, `"path": "rpm"`, `"path": ""`, 1), `field "rpm": path "" is not a plain field name`},
		{"a path that is not a plain field name", strings.Replace(fanSpeedSchema, `"path": "rpm"`, `"path": "../../etc"`, 1), `path "../../etc" is not a plain field name`},
		{"a field name that is not a plain name", strings.Replace(fanSpeedSchema, `"rpm": {`, `"r.pm": {`, 1), `field "r.pm": the field name is not a plain name`},
		{"an enum on a number field", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "type": "number", "enum": ["1"]`, 1), "enum is allowed only on a string field"},
		{"an enum on an untyped field", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "enum": ["1"]`, 1), "enum is allowed only on a string field"},
		{"an empty enum", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "type": "string", "enum": []`, 1), "enum must list at least one value"},
		{"a ref declared twice", strings.Replace(fanSpeedSchema, `}}}}`, `}}}, "fan.speed.observed/1.0": {"event_type": "fan.speed.observed", "schema_version": "1.0", "fields": {}}}`, 1), `duplicate key "fan.speed.observed/1.0"`},
		{"a field attribute declared twice", strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "rpm", "unit": "hz"`, 1), `duplicate key "unit"`},
		{"a null document", "null", "the document declares no event schema"},
		{"an empty object", "{}", "the document declares no event schema"},
		{"an empty file", "", "decode event schemas bad.json: read schema document"},
		{"a document nested too deeply", `{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"i":{"j":{}}}}}}}}}}}`, "nests deeper than 8 levels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.BuiltinEventSchemas().Merge("bad.json", []byte(tt.document))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Merge = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEveryBuiltInSchemaPassesTheRulesADirectorySchemaMust(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("event_schema_data.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.BuiltinEventSchemas().Merge("event_schema_data.json", data); err != nil {
		t.Fatalf("a built-in schema breaks the directory rules: %v", err)
	}
}

func TestACompiledSpecUsesTheRegistryItWasCompiledAgainst(t *testing.T) {
	t.Parallel()
	source := editedSpec("eventType: sensor.temperature", "eventType: fan.speed.observed",
		"schema: sensor.temperature/1.0", "schema: fan.speed.observed/1.0", "field: data.value", "field: data.rpm")
	if _, err := compileSource(t, source); err == nil || !strings.Contains(err.Error(), `unknown event schema "fan.speed.observed/1.0"`) {
		t.Fatalf("compile against the built-ins = %v, want the fan schema unknown", err)
	}
	merged, err := domain.BuiltinEventSchemas().Merge("fan.json", []byte(fanSpeedSchema))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := domain.NewCompiler().WithEventSchemas(merged).CompileBytes(t.Context(), []byte(source), "fan.yaml")
	if err != nil {
		t.Fatalf("compile against the merged registry: %v", err)
	}
	if _, ok := compiled.EventSchema("fan.speed.observed/1.0"); !ok {
		t.Fatal("the compiled spec lost the registry it was compiled against")
	}
	var loaded domain.CompiledSpec
	if _, ok := loaded.EventSchema("zone.temp.observed/1.0"); !ok {
		t.Fatal("a spec without a registry must fall back to the built-in schemas")
	}
}
