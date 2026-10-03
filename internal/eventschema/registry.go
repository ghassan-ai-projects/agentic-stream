// Package eventschema contains the deterministic catalog used by the spec
// compiler. Durable schema registrations are persisted separately.
package eventschema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

//go:embed registry_data.json
var registryData []byte

// Field describes one payload field exposed to deterministic operators.
type Field struct {
	Path     string
	Unit     string
	Type     string
	Optional bool
	Enum     []string
}

// Definition identifies a schema bound to one normalized event type.
type Definition struct {
	Ref           string
	EventType     string
	SchemaVersion string
	Fields        map[string]Field
}

// The event-schema catalog (motor/sensor/pump/pond/bay) lives in
// registry_data.json — domain DATA, not code (see
// docs/design/impl/GO_DOMAIN_DATA_EXTRACTION.md). Loaded once at first use.
var builtins = sync.OnceValues(loadRegistry)

func loadRegistry() (map[string]Definition, error) {
	var document map[string]registryEntry
	if err := json.Unmarshal(registryData, &document); err != nil {
		return nil, fmt.Errorf("decode eventschema registry data: %w", err)
	}
	registry := make(map[string]Definition, len(document))
	for ref, entry := range document {
		registry[ref] = entry.definition(ref)
	}
	return registry, nil
}

// registryEntry is one schema in registry_data.json.
type registryEntry struct {
	EventType     string                   `json:"event_type"`
	SchemaVersion string                   `json:"schema_version"`
	Fields        map[string]registryField `json:"fields"`
}

// registryField is one payload field in registry_data.json.
type registryField struct {
	Path     string   `json:"path"`
	Unit     string   `json:"unit"`
	Type     string   `json:"type"`
	Optional bool     `json:"optional"`
	Enum     []string `json:"enum"`
}

func (e registryEntry) definition(ref string) Definition {
	fields := make(map[string]Field, len(e.Fields))
	for name, field := range e.Fields {
		fields[name] = Field(field)
	}
	return Definition{Ref: ref, EventType: e.EventType, SchemaVersion: e.SchemaVersion, Fields: fields}
}

// Lookup returns a registered built-in definition.
func Lookup(ref string) (Definition, bool) {
	registry, err := builtins()
	if err != nil {
		// The catalog is embedded at build time; a decode failure is a
		// programming error and must fail loudly, not masquerade as an
		// unregistered ref.
		panic(fmt.Sprintf("eventschema: corrupt embedded registry data: %v", err))
	}
	definition, ok := registry[ref]
	return definition, ok
}

// JSON returns the structural schema for a built-in definition.
func JSON(definition Definition) ([]byte, error) {
	result, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": definition.schemaProperties(), "required": definition.requiredFields(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event schema: %w", err)
	}
	return result, nil
}

func (d Definition) schemaProperties() map[string]map[string]any {
	properties := make(map[string]map[string]any, len(d.Fields))
	for name, field := range d.Fields {
		properties[name] = field.schemaProperty()
	}
	return properties
}

// requiredFields lists the non-optional field names in sorted order.
func (d Definition) requiredFields() []string {
	required := make([]string, 0, len(d.Fields))
	for name, field := range d.Fields {
		if !field.Optional {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	return required
}

// schemaProperty is the field's JSON Schema property; untyped fields are
// numbers.
func (f Field) schemaProperty() map[string]any {
	fieldType := f.Type
	if fieldType == "" {
		fieldType = "number"
	}
	property := map[string]any{"type": fieldType}
	if f.Enum != nil {
		property["enum"] = f.Enum
	}
	return property
}
