package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed event_schema_data.json
var registryData []byte

// EventField describes one payload field exposed to deterministic operators.
type EventField struct {
	Path     string
	Unit     string
	Type     string
	Optional bool
	Enum     []string
}

// EventSchema identifies a schema bound to one normalized event type.
type EventSchema struct {
	Ref           string
	EventType     string
	SchemaVersion string
	Fields        map[string]EventField
}

// registryEntry is one schema in event_schema_data.json.
type registryEntry struct {
	EventType     string                   `json:"event_type"`
	SchemaVersion string                   `json:"schema_version"`
	Fields        map[string]registryField `json:"fields"`
}

// registryField is one payload field in event_schema_data.json.
type registryField struct {
	Path     string   `json:"path"`
	Unit     string   `json:"unit"`
	Type     string   `json:"type"`
	Optional bool     `json:"optional"`
	Enum     []string `json:"enum"`
}

func (e registryEntry) definition(ref string) EventSchema {
	fields := make(map[string]EventField, len(e.Fields))
	for name, field := range e.Fields {
		fields[name] = EventField(field)
	}
	return EventSchema{Ref: ref, EventType: e.EventType, SchemaVersion: e.SchemaVersion, Fields: fields}
}

// JSON returns the structural schema for a built-in definition.
func EventSchemaJSON(definition EventSchema) ([]byte, error) {
	result, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": definition.schemaProperties(), "required": definition.requiredFields(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event schema: %w", err)
	}
	return result, nil
}

func (d EventSchema) schemaProperties() map[string]map[string]any {
	properties := make(map[string]map[string]any, len(d.Fields))
	for name, field := range d.Fields {
		properties[name] = field.schemaProperty()
	}
	return properties
}

// requiredFields lists the non-optional field names in sorted order.
func (d EventSchema) requiredFields() []string {
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
func (f EventField) schemaProperty() map[string]any {
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
