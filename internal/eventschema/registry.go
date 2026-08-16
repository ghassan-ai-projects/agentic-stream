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
	var document map[string]struct {
		EventType     string `json:"event_type"`
		SchemaVersion string `json:"schema_version"`
		Fields        map[string]struct {
			Path     string `json:"path"`
			Unit     string `json:"unit"`
			Type     string `json:"type"`
			Optional bool   `json:"optional"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(registryData, &document); err != nil {
		return nil, fmt.Errorf("decode eventschema registry data: %w", err)
	}
	registry := make(map[string]Definition, len(document))
	for ref, def := range document {
		fields := make(map[string]Field, len(def.Fields))
		for name, field := range def.Fields {
			fields[name] = Field{Path: field.Path, Unit: field.Unit, Type: field.Type, Optional: field.Optional}
		}
		registry[ref] = Definition{
			Ref:           ref,
			EventType:     def.EventType,
			SchemaVersion: def.SchemaVersion,
			Fields:        fields,
		}
	}
	return registry, nil
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
	properties := make(map[string]map[string]string, len(definition.Fields))
	required := make([]string, 0, len(definition.Fields))
	for name, field := range definition.Fields {
		fieldType := field.Type
		if fieldType == "" {
			fieldType = "number"
		}
		properties[name] = map[string]string{"type": fieldType}
		if !field.Optional {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	result, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "properties": properties, "required": required,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event schema: %w", err)
	}
	return result, nil
}
